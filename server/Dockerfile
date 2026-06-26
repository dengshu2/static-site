# ── 构建阶段 ──────────────────────────────────────────────
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
# 纯标准库，无需 go.sum / go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /deployer .

# ── 运行阶段 ──────────────────────────────────────────────
FROM scratch
COPY --from=build /deployer /deployer
# 以非 root（uid=gid=1001，与宿主 dengshu 对齐）运行；
# 因此监听非特权端口 8080，数据卷文件归宿主用户所有。
USER 1001:1001
EXPOSE 8080
ENTRYPOINT ["/deployer"]
