package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	TelegramBotToken       string
	AllowedChatIDs         []int64
	AdminAuthKey           string
	DefaultProfile         string
	ProfilesRoot           string
	AgyBinPath             string
	WorkspaceRoot          string
	ExecutionTimeout       time.Duration
	SessionFilePath        string
	RateLimitPerMinute     int
	SessionExpirationHours int
}

func LoadConfig() (*Config, error) {
	_ = loadDotEnv(".env")

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is mandatory but missing")
	}

	allowedIDs := parseChatIDs(os.Getenv("ALLOWED_CHAT_IDS"))

	adminKey := os.Getenv("ADMIN_AUTH_KEY")

	defaultProfile := os.Getenv("DEFAULT_PROFILE")
	if defaultProfile == "" {
		defaultProfile = "agy01"
	}

	profilesRoot := os.Getenv("PROFILES_ROOT")
	if profilesRoot == "" {
		profilesRoot = "/opt/data/home/agy-profiles"
	}

	agyBinPath := os.Getenv("AGY_BIN_PATH")
	if agyBinPath == "" {
		agyBinPath = "/opt/data/home/.local/bin/agy"
	}

	workspaceRoot := os.Getenv("WORKSPACE_ROOT")
	if workspaceRoot == "" {
		workspaceRoot = "/opt/data/home"
	}

	timeoutStr := os.Getenv("EXECUTION_TIMEOUT")
	execTimeout := 30 * time.Minute
	if timeoutStr != "" {
		if parsed, err := time.ParseDuration(timeoutStr); err == nil && parsed > 0 {
			execTimeout = parsed
		}
	}

	sessionFilePath := os.Getenv("SESSION_FILE_PATH")
	if sessionFilePath == "" {
		sessionFilePath = "data/sessions.json"
	}

	rateLimit := 30
	if rateLimitStr := os.Getenv("RATE_LIMIT_PER_MINUTE"); rateLimitStr != "" {
		if parsed, err := strconv.Atoi(rateLimitStr); err == nil && parsed > 0 {
			rateLimit = parsed
		}
	}

	expirationHours := 72
	if expStr := os.Getenv("SESSION_EXPIRATION_HOURS"); expStr != "" {
		if parsed, err := strconv.Atoi(expStr); err == nil && parsed > 0 {
			expirationHours = parsed
		}
	}

	return &Config{
		TelegramBotToken:       token,
		AllowedChatIDs:         allowedIDs,
		AdminAuthKey:           adminKey,
		DefaultProfile:         defaultProfile,
		ProfilesRoot:           profilesRoot,
		AgyBinPath:             agyBinPath,
		WorkspaceRoot:          workspaceRoot,
		ExecutionTimeout:       execTimeout,
		SessionFilePath:        sessionFilePath,
		RateLimitPerMinute:     rateLimit,
		SessionExpirationHours: expirationHours,
	}, nil
}

func parseChatIDs(raw string) []int64 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	parts := strings.Split(trimmed, ",")
	var result []int64
	for _, part := range parts {
		clean := strings.TrimSpace(part)
		if clean == "" {
			continue
		}
		if id, err := strconv.ParseInt(clean, 10, 64); err == nil {
			result = append(result, id)
		}
	}
	return result
}

func loadDotEnv(filepath string) error {
	file, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)

		if os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}

	return scanner.Err()
}
