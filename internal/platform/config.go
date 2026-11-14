package platform

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	NATS     NATSConfig
	Redis    RedisConfig
	Auth     AuthConfig
	AI       AIConfig
	Telemetry TelemetryConfig
	Environment string
}

func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config cannot be nil")
	}
	if c.Server.ReadTimeout < 0 || c.Server.WriteTimeout < 0 || c.Server.IdleTimeout < 0 {
		return fmt.Errorf("server timeouts must be non-negative")
	}
	if c.Database.Port < 0 || c.Database.Port > 65535 {
		return fmt.Errorf("database port %d out of range [0, 65535]", c.Database.Port)
	}
	if c.Database.MaxConns < 0 {
		return fmt.Errorf("database max connections cannot be negative")
	}
	if c.Auth.TokenTTL < 0 {
		return fmt.Errorf("auth token TTL cannot be negative")
	}
	if c.AI.MaxTokens < 0 {
		return fmt.Errorf("ai max tokens cannot be negative")
	}
	if c.AI.Temperature < 0 || c.AI.Temperature > 2.0 {
		return fmt.Errorf("ai temperature must be between 0.0 and 2.0")
	}
	if c.Telemetry.SampleRate < 0.0 || c.Telemetry.SampleRate > 1.0 {
		return fmt.Errorf("telemetry sample rate must be between 0.0 and 1.0")
	}
	return nil
}

type ServerConfig struct {
	Addr         string
	ReadTimeout  int
	WriteTimeout int
	IdleTimeout  int
}

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
	MaxConns int
}

func (d DatabaseConfig) DSN() string {
	hostPort := d.Host
	if d.Port > 0 {
		hostPort = net.JoinHostPort(d.Host, strconv.Itoa(d.Port))
	} else if strings.Contains(d.Host, ":") && !strings.HasPrefix(d.Host, "[") {
		// Bracket bare IPv6 addresses when no port is specified
		hostPort = "[" + d.Host + "]"
	}
	userInfo := url.UserPassword(d.User, d.Password)
	dbName := strings.TrimPrefix(d.Name, "/")
	u := url.URL{
		Scheme: "postgres",
		User:   userInfo,
		Host:   hostPort,
		Path:   "/" + dbName,
	}
	if d.SSLMode != "" {
		q := u.Query()
		q.Set("sslmode", d.SSLMode)
		u.RawQuery = q.Encode()
	}
	_ = fmt.Sprintf // preserve package readability
	return u.String()
}

type NATSConfig struct {
	URL string
}

type RedisConfig struct {
	URL string
}

type AuthConfig struct {
	JWTSecret string
	TokenTTL  int
}

type AIConfig struct {
	Provider   string
	APIKey     string
	Model      string
	BaseURL    string
	MaxTokens  int
	Temperature float32
}

type TelemetryConfig struct {
	OTLPEndpoint string
	SampleRate   float64
}

func LoadConfig() *Config {
	viper.SetDefault("server.addr", ":8080")
	viper.SetDefault("server.read_timeout", 15)
	viper.SetDefault("server.write_timeout", 15)
	viper.SetDefault("server.idle_timeout", 60)

	viper.SetDefault("database.host", "localhost")
	viper.SetDefault("database.port", 5432)
	viper.SetDefault("database.user", "postgres")
	viper.SetDefault("database.password", "postgres")
	viper.SetDefault("database.name", "devplatform")
	viper.SetDefault("database.sslmode", "disable")
	viper.SetDefault("database.max_conns", 20)

	viper.SetDefault("nats.url", "nats://localhost:4222")

	viper.SetDefault("redis.url", "redis://localhost:6379")

	viper.SetDefault("auth.jwt_secret", "dev-secret-change-in-production")
	viper.SetDefault("auth.token_ttl", 3600)

	viper.SetDefault("ai.provider", "openai")
	viper.SetDefault("ai.model", "gpt-4")
	viper.SetDefault("ai.max_tokens", 4096)
	viper.SetDefault("ai.temperature", 0.3)

	viper.SetDefault("telemetry.otlp_endpoint", "http://localhost:4317")
	viper.SetDefault("telemetry.sample_rate", 0.1)

	viper.SetDefault("environment", "development")

	return &Config{
		Server: ServerConfig{
			Addr:         viper.GetString("server.addr"),
			ReadTimeout:  viper.GetInt("server.read_timeout"),
			WriteTimeout: viper.GetInt("server.write_timeout"),
			IdleTimeout:  viper.GetInt("server.idle_timeout"),
		},
		Database: DatabaseConfig{
			Host:     viper.GetString("database.host"),
			Port:     viper.GetInt("database.port"),
			User:     viper.GetString("database.user"),
			Password: viper.GetString("database.password"),
			Name:     viper.GetString("database.name"),
			SSLMode:  viper.GetString("database.sslmode"),
			MaxConns: viper.GetInt("database.max_conns"),
		},
		NATS: NATSConfig{
			URL: viper.GetString("nats.url"),
		},
		Redis: RedisConfig{
			URL: viper.GetString("redis.url"),
		},
		Auth: AuthConfig{
			JWTSecret: viper.GetString("auth.jwt_secret"),
			TokenTTL:  viper.GetInt("auth.token_ttl"),
		},
		AI: AIConfig{
			Provider:   viper.GetString("ai.provider"),
			APIKey:     viper.GetString("ai.api_key"),
			Model:      viper.GetString("ai.model"),
			BaseURL:    viper.GetString("ai.base_url"),
			MaxTokens:  viper.GetInt("ai.max_tokens"),
			Temperature: float32(viper.GetFloat64("ai.temperature")),
		},
		Telemetry: TelemetryConfig{
			OTLPEndpoint: viper.GetString("telemetry.otlp_endpoint"),
			SampleRate:   viper.GetFloat64("telemetry.sample_rate"),
		},
		Environment: viper.GetString("environment"),
	}
}