package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port             string
	MongoURI         string
	DBName           string
	JWTSecret        string
	AllowedOrigins   string
	AllowMethods     string
	AllowHeaders     string
	ExposeHeaders    string
	AllowCredentials bool
	DevJWTBypass     bool
	Environment      string
}

func Load() *Config {

	// Load .env only in non-production
	if os.Getenv("ENV") != "production" {
		if err := godotenv.Load(); err != nil {
			log.Println("No .env file found")
		}
	}
	// 29-may-2026 coomented this line because saare user ko same data show ho raha tha
	// devBypass, _ := strconv.ParseBool(getEnv("DEV_JWT_BYPASS", "false"))
	allowCreds, _ := strconv.ParseBool(getEnv("ALLOW_CREDENTIALS", "true"))

	cfg := &Config{
		Port:           getEnv("PORT", "8080"),
		MongoURI:       getEnv("MONGO_URI", ""),
		DBName:         getEnv("DB_NAME", "ap-dashboard"),
		JWTSecret:      getEnv("JWT_SECRET", ""),
		AllowedOrigins: getEnv("ALLOWED_ORIGINS", "http://localhost:5173"),
		// AllowedOrigins:   getEnv("ALLOWED_ORIGINS", ""),
		AllowMethods:     getEnv("ALLOW_METHODS", "GET,POST,PUT,PATCH,DELETE,OPTIONS"),
		AllowHeaders:     getEnv("ALLOW_HEADERS", "Origin,Content-Type,Accept,Authorization"),
		ExposeHeaders:    getEnv("EXPOSE_HEADERS", "Content-Length"),
		AllowCredentials: allowCreds,
		// 29-may-2026 coomented this line because saare user ko same data show ho raha tha
		// DevJWTBypass:     devBypass,
		Environment: getEnv("ENV", "development"),
	}

	validateCriticalEnv(cfg)

	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func validateCriticalEnv(cfg *Config) {
	if cfg.MongoURI == "" {
		log.Fatal("MONGO_URI is required")
	}
	log.Println("MongoURI:", cfg.MongoURI)
	if cfg.JWTSecret == "" && cfg.Environment == "production" {
		log.Fatal("JWT_SECRET is required in production")
	}

}
