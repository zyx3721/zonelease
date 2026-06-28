package router

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"zonelease/backend/internal/repository"
	authsvc "zonelease/backend/internal/service/auth"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Provider string `json:"provider"`
}

type resetVerifyRequest struct {
	Username      string `json:"username"`
	CaptchaToken  string `json:"captchaToken"`
	CaptchaAnswer string `json:"captchaAnswer"`
}

type resetVerifyResponse struct {
	VerificationToken string `json:"verificationToken"`
	Channels          any    `json:"channels"`
}

type resetSendRequest struct {
	Username          string `json:"username"`
	VerificationToken string `json:"verificationToken"`
	Channel           string `json:"channel"`
	VerifyEmail       string `json:"verifyEmail"`
	To                string `json:"to"`
}

type resetSendResponse struct {
	CooldownSeconds int    `json:"cooldownSeconds"`
	DevCode         string `json:"devCode,omitempty"`
}

type resetConfirmRequest struct {
	Username          string `json:"username"`
	VerificationToken string `json:"verificationToken"`
	Code              string `json:"code"`
	NewPassword       string `json:"newPassword"`
	ConfirmPassword   string `json:"confirmPassword"`
}

type changePasswordRequest struct {
	OldPassword     string `json:"old_password"`
	NewPassword     string `json:"new_password"`
	ConfirmPassword string `json:"confirm_password"`
}

func (r *Router) login(w http.ResponseWriter, req *http.Request) {
	var body loginRequest
	if !decode(w, req, &body) {
		return
	}
	if strings.TrimSpace(body.Username) == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "invalid_login", "用户名或密码不能为空")
		return
	}
	provider := strings.TrimSpace(body.Provider)
	if provider == "" {
		provider = "local"
	}
	session, err := r.auth.LoginWithProvider(req.Context(), provider, body.Username, body.Password)
	if err != nil {
		if errors.Is(err, authsvc.ErrUserNotProvisioned) {
			writeError(w, http.StatusUnauthorized, "user_not_provisioned", "用户未在平台中启用")
			return
		}
		writeError(w, statusFromErr(err), "login_failed", "用户名或密码错误")
		return
	}
	_ = r.store.WriteAudit(req.Context(), session.User.ID, session.User.Username, "User login", session.User.Username, "System", "success", auditMetadata(map[string]any{
		"username": session.User.Username,
		"provider": provider,
	}), repository.ClientIP(req))
	writeJSON(w, http.StatusOK, session)
}

func (r *Router) logout(w http.ResponseWriter, req *http.Request) {
	token := bearerToken(req)
	session, sessionErr := r.auth.Validate(req.Context(), token)
	if sessionErr == nil {
		_ = r.store.WriteAudit(req.Context(), session.User.ID, session.User.Username, "User logout", session.User.Username, "System", "success", auditMetadata(map[string]any{
			"username": session.User.Username,
			"provider": session.Provider,
		}), repository.ClientIP(req))
	}
	if err := r.auth.Logout(req.Context(), token); err != nil {
		r.logger.Error("Logout failed", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (r *Router) me(w http.ResponseWriter, req *http.Request) {
	session, err := r.auth.Validate(req.Context(), bearerToken(req))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "请先登录")
		return
	}
	writeJSON(w, http.StatusOK, session.User)
}

func (r *Router) passwordResetCaptcha(w http.ResponseWriter, req *http.Request) {
	captcha, err := r.auth.CreateCaptcha(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "captcha_failed", "生成验证码失败")
		return
	}
	writeJSON(w, http.StatusOK, captcha)
}

func (r *Router) passwordResetVerify(w http.ResponseWriter, req *http.Request) {
	var body resetVerifyRequest
	if !decode(w, req, &body) {
		return
	}
	token, channels, err := r.auth.VerifyResetIdentity(req.Context(), body.Username, body.CaptchaToken, body.CaptchaAnswer)
	if err != nil {
		switch {
		case errors.Is(err, authsvc.ErrInvalidResetCaptcha):
			writeError(w, http.StatusBadRequest, "invalid_captcha", "验证码不正确")
		case errors.Is(err, authsvc.ErrResetUnavailable):
			writeError(w, http.StatusBadRequest, "password_reset_unavailable", "当前账号无法找回密码")
		case errors.Is(err, authsvc.ErrResetChannelMissing):
			writeError(w, http.StatusServiceUnavailable, "no_password_reset_channel", "当前没有可用的找回密码媒介")
		default:
			writeError(w, statusFromErr(err), "verify_failed", "校验失败，请稍后重试")
		}
		return
	}
	writeJSON(w, http.StatusOK, resetVerifyResponse{VerificationToken: token, Channels: channels})
}

func (r *Router) passwordResetSend(w http.ResponseWriter, req *http.Request) {
	var body resetSendRequest
	if !decode(w, req, &body) {
		return
	}
	cooldown, code, err := r.auth.SendResetCode(req.Context(), body.VerificationToken, body.Channel, body.VerifyEmail)
	if err != nil {
		message := "发送验证码失败"
		if errors.Is(err, authsvc.ErrResetEmailMismatch) {
			message = "验证邮箱与账号邮箱不一致"
		}
		if errors.Is(err, authsvc.ErrResetCodeCooldown) {
			var cooldownErr authsvc.ResetCodeCooldownError
			if errors.As(err, &cooldownErr) && cooldownErr.RemainingSeconds > 0 {
				writeError(w, http.StatusTooManyRequests, "password_reset_cooldown", "验证码已发送，请于 "+strconv.Itoa(cooldownErr.RemainingSeconds)+" 秒后再试")
				return
			}
			writeError(w, http.StatusTooManyRequests, "password_reset_cooldown", "验证码已发送，请稍后再试")
			return
		}
		if errors.Is(err, authsvc.ErrResetCodeRateLimited) {
			writeError(w, http.StatusTooManyRequests, "password_reset_limited", "验证码请求过于频繁，请稍后再试")
			return
		}
		writeError(w, statusFromErr(err), "send_failed", message)
		return
	}
	response := resetSendResponse{CooldownSeconds: cooldown}
	if r.cfg.Server.Mode != "release" {
		response.DevCode = code
	}
	writeJSON(w, http.StatusOK, response)
}

func (r *Router) passwordResetConfirm(w http.ResponseWriter, req *http.Request) {
	var body resetConfirmRequest
	if !decode(w, req, &body) {
		return
	}
	if len(body.NewPassword) < 6 || body.NewPassword != body.ConfirmPassword {
		writeError(w, http.StatusBadRequest, "invalid_password", "新密码至少 6 位且两次输入必须一致")
		return
	}
	err := r.auth.ConfirmReset(req.Context(), body.Username, body.VerificationToken, body.Code, body.NewPassword)
	if err != nil {
		code := "reset_failed"
		message := "密码重置失败"
		if errors.Is(err, authsvc.ErrResetCodeMismatch) {
			code = "invalid_reset_code"
			message = "验证码不正确"
		}
		if errors.Is(err, authsvc.ErrResetUnavailable) {
			code = "password_reset_unavailable"
			message = "当前账号无法找回密码"
		}
		if errors.Is(err, authsvc.ErrInvalidResetToken) {
			code = "invalid_reset_token"
			message = "请重新完成用户名和图形验证码校验"
		}
		writeError(w, statusFromErr(err), code, message)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (r *Router) changePassword(w http.ResponseWriter, req *http.Request) {
	var body changePasswordRequest
	if !decode(w, req, &body) {
		return
	}
	if len(body.OldPassword) < 6 || len(body.NewPassword) < 6 || body.NewPassword != body.ConfirmPassword {
		writeError(w, http.StatusBadRequest, "invalid_password", "密码至少 6 位且两次输入必须一致")
		return
	}
	if body.OldPassword == body.NewPassword {
		writeError(w, http.StatusBadRequest, "invalid_password", "新密码不能与旧密码相同")
		return
	}
	user := currentUser(req)
	if err := r.auth.ChangePassword(req.Context(), user.ID, body.OldPassword, body.NewPassword); err != nil {
		if errors.Is(err, authsvc.ErrOldPasswordMismatch) {
			writeError(w, http.StatusBadRequest, "invalid_old_password", "旧密码不正确")
			return
		}
		writeError(w, statusFromErr(err), "change_password_failed", "密码修改失败")
		return
	}
	r.writeAudit(req, "Changed password", user.Username, "System", "success", map[string]any{"username": user.Username})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
