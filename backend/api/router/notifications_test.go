package router

import "testing"

func TestSanitizeEmailNotificationConfigRetainsSavedPassword(t *testing.T) {
	config, err := sanitizeEmailNotificationConfig(
		map[string]any{
			"smtpHost":    "smtp.example.com",
			"smtpPort":    465,
			"username":    "zonelease@example.com",
			"password":    "",
			"from":        "zonelease@example.com",
			"fromName":    "ZoneLease",
			"hasPassword": true,
			"useTLS":      true,
		},
		map[string]any{"password": "saved-password"},
		true,
		false,
	)
	if err != nil {
		t.Fatalf("sanitizeEmailNotificationConfig returned error: %v", err)
	}
	if got := stringValue(config["password"]); got != "saved-password" {
		t.Fatalf("password = %q, want saved-password", got)
	}
	if _, ok := config["hasPassword"]; ok {
		t.Fatal("hasPassword marker should not be stored")
	}
}

func TestSanitizeEmailNotificationConfigRetainsSavedPasswordWithoutMarker(t *testing.T) {
	config, err := sanitizeEmailNotificationConfig(
		map[string]any{
			"smtpHost": "smtp.example.com",
			"smtpPort": 465,
			"username": "zonelease@example.com",
			"password": "",
			"from":     "zonelease@example.com",
			"fromName": "ZoneLease",
			"useTLS":   true,
		},
		map[string]any{"password": "saved-password"},
		true,
		false,
	)
	if err != nil {
		t.Fatalf("sanitizeEmailNotificationConfig returned error: %v", err)
	}
	if got := stringValue(config["password"]); got != "saved-password" {
		t.Fatalf("password = %q, want saved-password", got)
	}
}
