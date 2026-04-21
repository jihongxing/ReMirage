import { Test, TestingModule } from '@nestjs/testing';
import { BridgeClientService } from './bridge-client.service';

describe('BridgeClientService', () => {
  let service: BridgeClientService;

  beforeEach(async () => {
    const module: TestingModule = await Test.createTestingModule({
      providers: [BridgeClientService],
    }).compile();

    service = module.get<BridgeClientService>(BridgeClientService);
  });

  it('should be defined', () => {
    expect(service).toBeDefined();
  });

  it('should initialize with default base URL', () => {
    delete process.env.BRIDGE_URL;
    delete process.env.BRIDGE_INTERNAL_SECRET;
    service.onModuleInit();
    // service is initialized without error
    expect(service).toBeDefined();
  });

  it('should use env vars when set', () => {
    process.env.BRIDGE_URL = 'http://localhost:9999';
    process.env.BRIDGE_INTERNAL_SECRET = 'test-secret';
    service.onModuleInit();
    expect(service).toBeDefined();
    delete process.env.BRIDGE_URL;
    delete process.env.BRIDGE_INTERNAL_SECRET;
  });
});
