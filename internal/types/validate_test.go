package types

import "testing"

func TestValidateItemName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "my-skill", false},
		{"valid short", "ab", false},
		{"valid with numbers", "skill-v2", false},
		{"valid long", "abcdefghijklmnopqrstuvwxyz-0123456789", false},
		{"empty", "", true},
		{"single char", "a", true},
		{"starts with number", "1skill", true},
		{"starts with hyphen", "-skill", true},
		{"uppercase", "MySkill", true},
		{"spaces", "my skill", true},
		{"underscores", "my_skill", true},
		{"dots", "my.skill", true},
		{"slashes", "my/skill", true},
		{"special chars", "my@skill!", true},
		{"too long", "abcdefghijklmnopqrstuvwxyz-0123456789-abcdefghijklmnopqrstuvwxyz12", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateItemName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateItemName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
