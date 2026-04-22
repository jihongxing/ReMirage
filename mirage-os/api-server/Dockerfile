# 多阶段构建：builder 阶段 npm ci --production，运行阶段只拷贝 dist + 生产依赖
FROM node:20-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci
COPY . .
RUN npx prisma generate
RUN npm run build

# 生产依赖阶段
FROM node:20-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci --production

# 运行阶段
FROM node:20-alpine
WORKDIR /app
RUN addgroup -g 1001 -S nodejs && adduser -S nestjs -u 1001
COPY --from=builder /app/dist ./dist
COPY --from=deps /app/node_modules ./node_modules
COPY --from=builder /app/package.json .
COPY --from=builder /app/src/prisma ./src/prisma
USER nestjs
EXPOSE 3000
CMD ["node", "dist/main"]
