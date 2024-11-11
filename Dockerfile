# 使用 Go 官方镜像作为基础镜像
FROM golang:1.20-alpine AS builder

# 设置工作目录
WORKDIR /app

# 将源代码复制到工作目录中
COPY main.go .
COPY backend.go .
COPY host.go .

# 编译 Go 代码，生成二进制文件
RUN go build -o proxy_service .

# 创建一个更小的运行镜像
FROM alpine:latest

# 设置工作目录
WORKDIR /root/

# 从构建镜像中复制二进制文件
COPY --from=builder /app/proxy_service .

# 暴露端口
EXPOSE 3666

# 启动服务
CMD ["./proxy_service"]
