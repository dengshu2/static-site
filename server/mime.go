package main

import "mime"

// 镜像基于 scratch，没有 /etc/mime.types，Go 内置表也不认识 3D 模型格式：
// FileServer 只能嗅探内容，.usdz 会被当成 application/zip，加上 nosniff 后
// iPhone Safari 只会下载文件，不会打开 AR Quick Look。
func init() {
	for ext, typ := range map[string]string{
		".usdz": "model/vnd.usdz+zip",
		".glb":  "model/gltf-binary",
		".gltf": "model/gltf+json",
	} {
		if err := mime.AddExtensionType(ext, typ); err != nil {
			panic(err)
		}
	}
}
