import { Test, TestingModule } from '@nestjs/testing';
import { BridgeClientService } from './bridge-client.service';

describe('BridgeClientService', () => {
  let service: BridgeClientService;
  const originalEnv = process.env;

  beforeEach(async () => {
    process.env = { ...originalEnv };
    const module: TestingModule = await Test.createTestingModule({
      providers: [BridgeClientService],
    }).compile();

    service = module.get<BridgeClientService>(BridgeClientService);
  });

  afterEach(() => {
    process.env = originalEnv;
  });

  it('should be defined', () => {
    expect(service).toBeDefined();
  });

  it('should initialize with default base URL when internal secret is set', () => {
    delete process.env.BRIDGE_URL;
    process.env.BRIDGE_INTERNAL_SECRET = 'test-secret';
    service.onModuleInit();
    expect(service).toBeDefined();
  });

  it('should reject startup without internal secret', () => {
    delete process.env.BRIDGE_INTERNAL_SECRET;
    expect(() => service.onModuleInit()).toThrow(/BRIDGE_INTERNAL_SECRET/);
  });

  it('should use env vars when set', () => {
    process.env.BRIDGE_URL = 'http://localhost:9999';
    process.env.BRIDGE_INTERNAL_SECRET = 'test-secret';
    service.onModuleInit();
    expect(service).toBeDefined();
  });
});
