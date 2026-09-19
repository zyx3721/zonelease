-- 企业微信账号与平台用户的绑定关系，一个企业微信账号只能绑定一个用户
CREATE TABLE IF NOT EXISTS user_wecom_bindings (
  user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  wecom_userid TEXT NOT NULL UNIQUE,
  bound_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
