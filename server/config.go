package main

import (
	"log"
	"os"
	"strconv"
)

// Config 来自环境变量，集中在这里解析，避免散落各处。
type Config struct {
	Listen      string // 监听地址，如 ":80"
	DataDir     string // 数据根目录，站点与元数据都落在这里
	Token       string // 上传/管理接口的 Bearer Token
	MaxUploadMB int64  // 单次上传体积上限（MB）
	MaxUnzipMB  int64  // zip 解压后总大小上限（MB），防 zip bomb
}

func loadConfig() Config {
	c := Config{
		Listen:      env("LISTEN", ":8080"),
		DataDir:     env("DATA_DIR", "/data"),
		Token:       os.Getenv("DEPLOY_TOKEN"),
		MaxUploadMB: envInt("MAX_UPLOAD_MB", 50),
		MaxUnzipMB:  envInt("MAX_UNZIP_MB", 200),
	}
	if c.Token == "" {
		log.Fatal("DEPLOY_TOKEN 未设置：私有部署必须配置 Token，否则任何人都能上传")
	}
	return c
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}
