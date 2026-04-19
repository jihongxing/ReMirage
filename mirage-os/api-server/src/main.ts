import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module';

async function bootstrap() {
  const app = await NestFactory.create(AppModule);
  app.setGlobalPrefix('api');

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
