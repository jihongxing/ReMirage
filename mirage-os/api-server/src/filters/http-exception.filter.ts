import {
  ExceptionFilter,
  Catch,
  ArgumentsHost,
  HttpException,
} from '@nestjs/common';

@Catch()
export class HttpExceptionFilter implements ExceptionFilter {
  catch(exception: unknown, host: ArgumentsHost) {
    const ctx = host.switchToHttp();
    const response = ctx.getResponse();

    const status =
      exception instanceof HttpException ? exception.getStatus() : 500;

    const body: Record<string, unknown> = {
      statusCode: status,
      message: 'Internal server error',
    };

    if (exception instanceof HttpException) {
      const res = exception.getResponse();
      body.message =
        typeof res === 'string'
          ? res
          : (res as Record<string, unknown>).message || exception.message;
    }

    // 非生产模式附带堆栈信息便于调试
    if (process.env.NODE_ENV !== 'production' && exception instanceof Error) {
      body.stack = exception.stack;
    }

    response.status(status).json(body);
  }
}
