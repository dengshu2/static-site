package main

import (
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 来自环境变量，集中在这里解析，避免散落各处。
type Config struct {
	Listen         string
	DataDir        string
	Token          string
	PublicHost     string
	AdminHost      string
	ContentHost    string
	PublicBaseURL  string
	AdminBaseURL   string
	ContentBaseURL string
	MaxUploadMB    int64
	MaxUnzipMB     int64
	MaxTotalMB     int64
	MaxZipFiles    int
	MaxPathDepth   int
	MaxSiteNameLen int
	MaxSites       int
	TrashRetention time.Duration
	// ShotURL is the screenshot service that makes project covers; empty turns
	// covers off. InternalHost is the name the service reaches this server by:
	// requests to it get the projects only, and are not counted as visits.
	ShotURL      string
	InternalHost string
}

func loadConfig() Config {
	publicHost := env("PUBLIC_HOST", "site.dengshu.ovh")
	adminHost := env("ADMIN_HOST", publicHost)
	contentHost := env("CONTENT_HOST", publicHost)
	c := Config{
		Listen:         env("LISTEN", ":8080"),
		DataDir:        env("DATA_DIR", "/data"),
		Token:          os.Getenv("DEPLOY_TOKEN"),
		PublicHost:     publicHost,
		AdminHost:      adminHost,
		ContentHost:    contentHost,
		PublicBaseURL:  env("PUBLIC_BASE_URL", "https://"+publicHost),
		AdminBaseURL:   env("ADMIN_BASE_URL", defaultAdminBaseURL(publicHost, adminHost)),
		ContentBaseURL: env("CONTENT_BASE_URL", "https://"+contentHost),
		MaxUploadMB:    envPositiveInt64("MAX_UPLOAD_MB", 50),
		MaxUnzipMB:     envPositiveInt64("MAX_UNZIP_MB", 200),
		MaxTotalMB:     envPositiveInt64("MAX_TOTAL_MB", 10*1024),
		MaxZipFiles:    int(envPositiveInt64("MAX_ZIP_FILES", 5000)),
		MaxPathDepth:   int(envPositiveInt64("MAX_PATH_DEPTH", 20)),
		MaxSiteNameLen: int(envPositiveInt64("MAX_SITE_NAME_LEN", 63)),
		MaxSites:       int(envPositiveInt64("MAX_SITES", 1000)),
		TrashRetention: time.Duration(envPositiveInt64("TRASH_RETENTION_HOURS", 168)) * time.Hour,
		ShotURL:        env("SHOT_URL", ""),
		InternalHost:   strings.ToLower(env("INTERNAL_HOST", "")),
	}
	if c.Token == "" {
		log.Fatal("DEPLOY_TOKEN 未设置：私有部署必须配置 Token，否则任何人都能上传")
	}
	if len(c.Token) < 7 {
		log.Fatal("DEPLOY_TOKEN 至少需要 7 个字符")
	}
	if c.MaxSiteNameLen < 8 || c.MaxSiteNameLen > 120 {
		log.Fatal("MAX_SITE_NAME_LEN 必须在 8 到 120 之间")
	}
	if c.MaxUploadMB > c.MaxUnzipMB {
		log.Fatal("MAX_UNZIP_MB 不能小于 MAX_UPLOAD_MB")
	}
	for key, raw := range map[string]string{
		"PUBLIC_BASE_URL":  c.PublicBaseURL,
		"ADMIN_BASE_URL":   c.AdminBaseURL,
		"CONTENT_BASE_URL": c.ContentBaseURL,
	} {
		u, err := url.Parse(raw)
		localHTTP := u != nil && u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost")
		if err != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "https" && !localHTTP {
			log.Fatalf("%s 必须是无查询参数的 HTTPS URL（本地回环地址可使用 HTTP）", key)
		}
	}
	return c
}

func defaultAdminBaseURL(publicHost, adminHost string) string {
	if adminHost == publicHost {
		return "https://" + publicHost + "/admin"
	}
	return "https://" + adminHost
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return strings.TrimRight(v, "/")
	}
	return def
}

func envPositiveInt64(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
		log.Fatalf("%s 必须是正整数", key)
	}
	return def
}
