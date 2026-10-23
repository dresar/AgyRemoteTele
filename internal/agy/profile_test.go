package agy

import (
	"strings"
	"testing"
)

func TestProfileManager(t *testing.T) {
	pm := NewProfileManager("/opt/data/home/agy-profiles", "agy01")

	if !pm.ValidateProfile("agy01") {
		t.Error("expected agy01 to be valid")
	}
	if !pm.ValidateProfile("agy04") {
		t.Error("expected agy04 to be valid")
	}
	if pm.ValidateProfile("agy99") {
		t.Error("expected agy99 to be invalid")
	}
	if pm.ValidateProfile("invalid") {
		t.Error("expected invalid to be rejected")
	}

	env := pm.BuildEnv("agy04")
	foundHome := false
	foundGemini := false
	for _, e := range env {
		if strings.HasPrefix(e, "HOME=/opt/data/home/agy-profiles/agy04") {
			foundHome = true
		}
		if strings.HasPrefix(e, "GEMINI_HOME=/opt/data/home/agy-profiles/agy04/.gemini") {
			foundGemini = true
		}
	}

	if !foundHome {
		t.Error("expected HOME environment to point to agy04 profile home")
	}
	if !foundGemini {
		t.Error("expected GEMINI_HOME environment to point to agy04 gemini dir")
	}
}
