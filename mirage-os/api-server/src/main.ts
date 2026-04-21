import { NestFactory } from '@nestjs/core';
import { ValidationPipe } from '@nestjs/common';
import helmet from 'helmet';
import { AppModule } from './app.module';
import { HttpExceptionFilter } from './filters/http-exception.filter';

async function bootstrap() {
  // 生产模式校验 JWT_SECRET
  if (process.env.NODE_ENV === 'production' && !process.env.JWT_SECRET) {
    throw new Error('生产模式必须设置 JWT_SECRET 环境变量');
  }

  const app = await NestFactory.create(AppModule);
  app.setGlobalPrefix('api');

  // 5.1 输入校验
  app.useGlobalPipes(
    new ValidationPipe({
      whitelist: true,
      forbidNonWhitelisted: true,
      transform: true,
    }),
  );

  // 5.2 安全 HTTP Header
  app.use(
    helmet({
      contentSecurityPolicy: false,
      hsts: process.env.NODE_ENV === 'production',
    }),
  );

  // 5.3 CORS 收敛
  app.enableCors({
    origin:
      process.env.NODE_ENV === 'production'
        ? (process.env.ALLOWED_ORIGINS || '').split(',')
        : true,
    credentials: true,
  });

  // 5.4 全局异常过滤器（生产模式脱敏）
  app.useGlobalFilters(new HttpExceptionFilter());

  // 健康检查端点（不带 /api 前缀，供 docker healthcheck 使用）
  const httpAdapter = app.getHttpAdapter();
  httpAdapter.get('/health', (_req, res) => {
    res.status(200).send('OK');
  });

  const port = process.env.PORT || 3000;
  await app.listen(port);
  console.log(`[INFO] api-server listening on :${port}`);
}
bootstrap();
