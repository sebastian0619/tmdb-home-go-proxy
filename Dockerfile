# 使用 Go 官方镜像作为基础镜像
FROM golang:1.20-alpine AS builder

# 设置工作目录
WORKDIR /app

# 复制代码文件到镜像中
COPY proxy_server.go .

# 编译 Go 代码，生成二进制文件
RUN go build -o proxy_server proxy_server.go

# 创建一个更小的运行镜像
FROM alpine:latest

# 设置工作目录
WORKDIR /root/

# 从构建镜像中复制二进制文件
COPY --from=builder /app/proxy_server .

# 设置端口
EXPOSE 3666

# 启动代理服务器
CMD ["./proxy_server"]
