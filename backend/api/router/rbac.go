package router

import (
	"errors"
	"net/http"
	"net/mail"
	"regexp"
	"strings"

	"zonelease/backend/internal/domain"
	"zonelease/backend/internal/repository"
)

var roleKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{1,40}$`)

type managedUserRequest struct {
	Username    string   `json:"username"`
	Password    string   `json:"password"`
	Email       string   `json:"email"`
	DisplayName string   `json:"displayName"`
	RoleKeys    []string `json:"roleKeys"`
	Disabled    bool     `json:"disabled"`
}

type roleRequest struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

type userGroupRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Disabled    bool     `json:"disabled"`
	MemberIDs   []string `json:"memberIds"`
	RoleKeys    []string `json:"roleKeys"`
}

func (r *Router) listManagedUsers(w http.ResponseWriter, req *http.Request) {
	items, err := r.store.ListManagedUsers(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_users_failed", "读取用户列表失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (r *Router) createManagedUser(w http.ResponseWriter, req *http.Request) {
	input, ok := r.decodeManagedUserRequest(w, req, true)
	if !ok {
		return
	}
	passwordHash, err := repository.HashPassword(input.Password)
	if err != nil {
		r.logger.Error("Hash managed user password failed", "error", err)
		writeError(w, http.StatusInternalServerError, "create_user_failed", "创建用户失败")
		return
	}
	user, err := r.store.CreateManagedUser(req.Context(), repository.UserInput{
		Username: input.Username, Email: input.Email, DisplayName: input.DisplayName, RoleKeys: input.RoleKeys, Disabled: input.Disabled,
	}, passwordHash)
	if err != nil {
		if repository.IsUniqueViolation(err) {
			writeError(w, http.StatusConflict, "user_exists", "用户名已存在")
			return
		}
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "role_not_found", "选择的角色不存在")
			return
		}
		r.logger.Error("Create managed user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "create_user_failed", "创建用户失败")
		return
	}
	r.writeAudit(req, "settings.user.create", user.ID, "System", "success", map[string]any{
		"username": user.Username,
		"email":    user.Email,
		"role":     user.Role,
		"disabled": user.Disabled,
	})
	writeJSON(w, http.StatusCreated, user)
}

func (r *Router) managedUserRoute(w http.ResponseWriter, req *http.Request) {
	id, action, ok := parseSettingsResourcePath(req.URL.Path, "/api/settings/users/")
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	if req.Method == http.MethodPut && action == "" {
		r.updateManagedUser(w, req, id)
		return
	}
	if req.Method == http.MethodDelete && action == "" {
		r.deleteManagedUser(w, req, id)
		return
	}
	if req.Method == http.MethodPost && action == "disabled" {
		r.setManagedUserDisabled(w, req, id)
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不支持")
}

func (r *Router) deleteManagedUser(w http.ResponseWriter, req *http.Request, id string) {
	current := currentUser(req)
	if id == current.ID {
		writeError(w, http.StatusBadRequest, "delete_self_forbidden", "不能删除当前登录用户")
		return
	}
	user, err := r.store.FindUserByID(req.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user_not_found", "用户不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "delete_user_failed", "删除用户失败")
		return
	}
	if isDefaultAdminUser(user) {
		writeError(w, http.StatusBadRequest, "default_admin_protected", "默认管理员不能删除")
		return
	}
	if !user.Disabled {
		writeError(w, http.StatusBadRequest, "user_must_be_disabled", "请先禁用用户再删除")
		return
	}
	if err := r.store.DeleteManagedUser(req.Context(), id); err != nil {
		writeError(w, statusFromErr(err), "delete_user_failed", "删除用户失败")
		return
	}
	r.writeAudit(req, "settings.user.delete", id, "System", "success", map[string]any{"username": user.Username, "email": user.Email})
	w.WriteHeader(http.StatusNoContent)
}

func (r *Router) updateManagedUser(w http.ResponseWriter, req *http.Request, id string) {
	input, ok := r.decodeManagedUserRequest(w, req, false)
	if !ok {
		return
	}
	current := currentUser(req)
	if id == current.ID && input.Disabled {
		writeError(w, http.StatusBadRequest, "disable_self_forbidden", "不能禁用当前登录用户")
		return
	}
	existing, err := r.store.FindUserByID(req.Context(), id)
	if err != nil {
		writeError(w, statusFromErr(err), "user_not_found", "用户不存在")
		return
	}
	if isDefaultAdminUser(existing) && (input.Disabled || input.Username != existing.Username) {
		writeError(w, http.StatusBadRequest, "default_admin_protected", "默认管理员不能禁用、删除或改名")
		return
	}
	passwordHash := ""
	if input.Password != "" {
		hash, err := repository.HashPassword(input.Password)
		if err != nil {
			r.logger.Error("Hash managed user password failed", "error", err)
			writeError(w, http.StatusInternalServerError, "update_user_failed", "保存用户失败")
			return
		}
		passwordHash = hash
	}
	user, err := r.store.UpdateManagedUser(req.Context(), id, repository.UserInput{
		Username: input.Username, Email: input.Email, DisplayName: input.DisplayName, RoleKeys: input.RoleKeys, Disabled: input.Disabled,
	}, passwordHash)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user_not_found", "用户或角色不存在")
			return
		}
		if repository.IsUniqueViolation(err) {
			writeError(w, http.StatusConflict, "user_exists", "用户名已存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "update_user_failed", "保存用户失败")
		return
	}
	r.writeAudit(req, "settings.user.update", user.ID, "System", "success", map[string]any{
		"username":        user.Username,
		"email":           user.Email,
		"role":            user.Role,
		"disabled":        user.Disabled,
		"passwordChanged": input.Password != "",
	})
	writeJSON(w, http.StatusOK, user)
}

func (r *Router) setManagedUserDisabled(w http.ResponseWriter, req *http.Request, id string) {
	var body struct {
		Disabled bool `json:"disabled"`
	}
	if !decode(w, req, &body) {
		return
	}
	current := currentUser(req)
	if id == current.ID && body.Disabled {
		writeError(w, http.StatusBadRequest, "disable_self_forbidden", "不能禁用当前登录用户")
		return
	}
	if body.Disabled {
		user, err := r.store.FindUserByID(req.Context(), id)
		if err != nil {
			writeError(w, statusFromErr(err), "user_not_found", "用户不存在")
			return
		}
		if isDefaultAdminUser(user) {
			writeError(w, http.StatusBadRequest, "default_admin_protected", "默认管理员不能禁用")
			return
		}
	}
	user, err := r.store.SetManagedUserDisabled(req.Context(), id, body.Disabled)
	if err != nil {
		writeError(w, statusFromErr(err), "update_user_failed", "更新用户状态失败")
		return
	}
	r.writeAudit(req, "settings.user.disabled", user.ID, "System", "success", map[string]any{"username": user.Username, "disabled": user.Disabled})
	writeJSON(w, http.StatusOK, user)
}

func (r *Router) decodeManagedUserRequest(w http.ResponseWriter, req *http.Request, create bool) (managedUserRequest, bool) {
	var input managedUserRequest
	if !decode(w, req, &input) {
		return managedUserRequest{}, false
	}
	input.Username = strings.TrimSpace(input.Username)
	input.Email = strings.TrimSpace(input.Email)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.RoleKeys = normalizeRequestStrings(input.RoleKeys)
	if input.Username == "" {
		writeError(w, http.StatusBadRequest, "invalid_username", "用户名不能为空")
		return managedUserRequest{}, false
	}
	if _, err := mail.ParseAddress(input.Email); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_email", "请输入有效的邮箱地址")
		return managedUserRequest{}, false
	}
	if input.DisplayName == "" {
		input.DisplayName = input.Username
	}
	if create && len(input.Password) < 6 {
		writeError(w, http.StatusBadRequest, "invalid_password", "密码至少 6 个字符")
		return managedUserRequest{}, false
	}
	if !create && input.Password != "" && len(input.Password) < 6 {
		writeError(w, http.StatusBadRequest, "invalid_password", "密码至少 6 个字符")
		return managedUserRequest{}, false
	}
	if len(input.RoleKeys) == 0 {
		input.RoleKeys = []string{"viewer"}
	}
	return input, true
}

func (r *Router) listRoles(w http.ResponseWriter, req *http.Request) {
	items, err := r.store.ListRoles(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_roles_failed", "读取用户角色失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (r *Router) createRole(w http.ResponseWriter, req *http.Request) {
	input, ok := decodeRoleRequest(w, req)
	if !ok {
		return
	}
	role, err := r.store.UpsertCustomRole(req.Context(), "", input)
	if err != nil {
		if repository.IsUniqueViolation(err) {
			writeError(w, http.StatusConflict, "role_exists", "角色标识已存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "save_role_failed", "保存用户角色失败")
		return
	}
	r.writeAudit(req, "settings.role.create", role.ID, "System", "success", map[string]any{
		"key":         role.Key,
		"name":        role.Name,
		"permissions": len(role.Permissions),
	})
	writeJSON(w, http.StatusCreated, role)
}

func (r *Router) roleRoute(w http.ResponseWriter, req *http.Request) {
	id, _, ok := parseSettingsResourcePath(req.URL.Path, "/api/settings/roles/")
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	if req.Method == http.MethodPut {
		input, ok := decodeRoleRequest(w, req)
		if !ok {
			return
		}
		role, err := r.store.UpsertCustomRole(req.Context(), id, input)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				writeError(w, http.StatusNotFound, "role_not_found", "内置角色不可修改或角色不存在")
				return
			}
			writeError(w, http.StatusInternalServerError, "save_role_failed", "保存用户角色失败")
			return
		}
		r.writeAudit(req, "settings.role.update", role.ID, "System", "success", map[string]any{
			"key":         role.Key,
			"name":        role.Name,
			"permissions": len(role.Permissions),
		})
		writeJSON(w, http.StatusOK, role)
		return
	}
	if req.Method == http.MethodDelete {
		if err := r.store.DeleteCustomRole(req.Context(), id); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				writeError(w, http.StatusNotFound, "role_not_found", "内置角色不可删除或角色不存在")
				return
			}
			writeError(w, http.StatusInternalServerError, "delete_role_failed", "删除用户角色失败")
			return
		}
		r.writeAudit(req, "settings.role.delete", id, "System", "success", map[string]any{"role": id})
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不支持")
}

func decodeRoleRequest(w http.ResponseWriter, req *http.Request) (repository.RoleInput, bool) {
	var input roleRequest
	if !decode(w, req, &input) {
		return repository.RoleInput{}, false
	}
	input.Key = strings.TrimSpace(input.Key)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Permissions = normalizeRequestStrings(input.Permissions)
	if !roleKeyPattern.MatchString(input.Key) {
		writeError(w, http.StatusBadRequest, "invalid_role_key", "角色标识需为小写字母、数字、点、下划线或连字符")
		return repository.RoleInput{}, false
	}
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_role_name", "角色名称不能为空")
		return repository.RoleInput{}, false
	}
	return repository.RoleInput{Key: input.Key, Name: input.Name, Description: input.Description, Permissions: input.Permissions}, true
}

func (r *Router) listUserGroups(w http.ResponseWriter, req *http.Request) {
	items, err := r.store.ListUserGroups(req.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_user_groups_failed", "读取用户群组失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func (r *Router) createUserGroup(w http.ResponseWriter, req *http.Request) {
	input, ok := decodeUserGroupRequest(w, req)
	if !ok {
		return
	}
	group, err := r.store.UpsertUserGroup(req.Context(), "", input)
	if err != nil {
		if repository.IsUniqueViolation(err) {
			writeError(w, http.StatusConflict, "user_group_exists", "用户群组已存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "save_user_group_failed", "保存用户群组失败")
		return
	}
	r.writeAudit(req, "settings.user_group.create", group.ID, "System", "success", map[string]any{
		"name":     group.Name,
		"members":  len(group.Members),
		"roles":    len(group.Roles),
		"disabled": group.Disabled,
	})
	writeJSON(w, http.StatusCreated, group)
}

func (r *Router) userGroupRoute(w http.ResponseWriter, req *http.Request) {
	id, _, ok := parseSettingsResourcePath(req.URL.Path, "/api/settings/user-groups/")
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		return
	}
	if req.Method == http.MethodPut {
		input, ok := decodeUserGroupRequest(w, req)
		if !ok {
			return
		}
		group, err := r.store.UpsertUserGroup(req.Context(), id, input)
		if err != nil {
			writeError(w, statusFromErr(err), "save_user_group_failed", "保存用户群组失败")
			return
		}
		r.writeAudit(req, "settings.user_group.update", group.ID, "System", "success", map[string]any{
			"name":     group.Name,
			"members":  len(group.Members),
			"roles":    len(group.Roles),
			"disabled": group.Disabled,
		})
		writeJSON(w, http.StatusOK, group)
		return
	}
	if req.Method == http.MethodDelete {
		if err := r.store.DeleteUserGroup(req.Context(), id); err != nil {
			writeError(w, statusFromErr(err), "delete_user_group_failed", "删除用户群组失败")
			return
		}
		r.writeAudit(req, "settings.user_group.delete", id, "System", "success", map[string]any{"group": id})
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不支持")
}

func decodeUserGroupRequest(w http.ResponseWriter, req *http.Request) (repository.UserGroupInput, bool) {
	var input userGroupRequest
	if !decode(w, req, &input) {
		return repository.UserGroupInput{}, false
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.MemberIDs = normalizeRequestStrings(input.MemberIDs)
	input.RoleKeys = normalizeRequestStrings(input.RoleKeys)
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "invalid_user_group_name", "用户群组名称不能为空")
		return repository.UserGroupInput{}, false
	}
	return repository.UserGroupInput{Name: input.Name, Description: input.Description, Disabled: input.Disabled, MemberIDs: input.MemberIDs, RoleKeys: input.RoleKeys}, true
}

func (r *Router) listPermissions(w http.ResponseWriter, req *http.Request) {
	items := make([]domain.Permission, len(repository.BuiltinPermissions))
	copy(items, repository.BuiltinPermissions)
	for index := range items {
		items[index].ImpliedReadPermission = repository.ImpliedReadPermissions[items[index].Key]
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func isDefaultAdminUser(user domain.User) bool {
	return user.Username == "admin"
}

func hasPermission(user domain.User, permission string) bool {
	if user.Role == "admin" {
		return true
	}
	for _, item := range user.Permissions {
		if item == permission {
			return true
		}
	}
	return false
}

func (r *Router) ensurePermission(w http.ResponseWriter, req *http.Request, permission string) bool {
	if hasPermission(currentUser(req), permission) {
		return true
	}
	writeError(w, http.StatusForbidden, "permission_denied", "当前用户无权执行此操作")
	return false
}

func (r *Router) requirePermission(permission string, next http.Handler) http.Handler {
	return r.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !r.ensurePermission(w, req, permission) {
			return
		}
		next.ServeHTTP(w, req)
	}))
}
