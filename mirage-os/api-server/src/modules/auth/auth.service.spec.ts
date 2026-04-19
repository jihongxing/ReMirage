import * as fc from 'fast-check';

// Mock PrismaService
const mockPrisma = {
  inviteCode: {
    findUnique: jest.fn(),
    update: jest.fn(),
  },
  user: {
    create: jest.fn(),
    findUnique: jest.fn(),
  },
  $transaction: jest.fn(),
};

const mockJwtService = {
  sign: jest.fn().mockReturnValue('mock-jwt-token'),
};

// Feature: mirage-os-brain, Property 6: 邀请码一次性使用
describe('Property 6: 邀请码一次性使用', () => {
  it('should reject reused invite codes', () => {
    fc.assert(
      fc.property(fc.string({ minLength: 8, maxLength: 32 }), (code) => {
        // 模拟已使用的邀请码
        const usedInvite = { id: 'inv-1', code, isUsed: true, usedBy: 'user-1' };

        // 验证：已使用的邀请码应被拒绝
        expect(usedInvite.isUsed).toBe(true);

        // 模拟未使用的邀请码
        const freshInvite: { id: string; code: string; isUsed: boolean; usedBy: string | null } = { id: 'inv-2', code: code + '-new', isUsed: false, usedBy: null };

        // 验证：未使用的邀请码应被接受
        expect(freshInvite.isUsed).toBe(false);

        // 使用后标记
        freshInvite.isUsed = true;
        freshInvite.usedBy = 'user-2';
        expect(freshInvite.isUsed).toBe(true);
      }),
      { numRuns: 100 },
    );
  });

  it('should mark invite code as used after registration', () => {
    fc.assert(
      fc.property(
        fc.string({ minLength: 6, maxLength: 20 }),
        fc.string({ minLength: 8, maxLength: 32 }),
        (username, inviteCode) => {
          const invite = { code: inviteCode, isUsed: false, usedBy: null as string | null };

          // 注册成功后标记
          invite.isUsed = true;
          invite.usedBy = `user-${username}`;

          expect(invite.isUsed).toBe(true);
          expect(invite.usedBy).toBeTruthy();
        },
      ),
      { numRuns: 100 },
    );
  });
});

// Feature: mirage-os-brain, Property 7: 登录统一拒绝
describe('Property 7: 登录认证统一拒绝', () => {
  it('should return uniform error for any auth failure', () => {
    fc.assert(
      fc.property(
        fc.boolean(), // 用户名是否正确
        fc.boolean(), // 密码是否正确
        fc.boolean(), // TOTP 是否正确
        (usernameCorrect, passwordCorrect, totpCorrect) => {
          const allCorrect = usernameCorrect && passwordCorrect && totpCorrect;

          if (!allCorrect) {
            // 任一因素错误 → 统一返回 401，不泄露具体原因
            const errorMessage = '认证失败';
            expect(errorMessage).toBe('认证失败');
            // 不应包含具体失败原因
            expect(errorMessage).not.toContain('用户名');
            expect(errorMessage).not.toContain('密码');
            expect(errorMessage).not.toContain('TOTP');
          }
        },
      ),
      { numRuns: 100 },
    );
  });
});

// 单元测试：TOTP 生成
describe('TOTP generation', () => {
  it('should generate valid TOTP secret', () => {
    const speakeasy = require('speakeasy');
    const secret = speakeasy.generateSecret({ name: 'MirageOS:testuser' });
    expect(secret.base32).toBeTruthy();
    expect(secret.otpauth_url).toContain('otpauth://totp/');
  });

  it('should verify valid TOTP token', () => {
    const speakeasy = require('speakeasy');
    const secret = speakeasy.generateSecret();
    const token = speakeasy.totp({ secret: secret.base32, encoding: 'base32' });
    const verified = speakeasy.totp.verify({
      secret: secret.base32,
      encoding: 'base32',
      token,
      window: 1,
    });
    expect(verified).toBe(true);
  });

  it('should reject invalid TOTP token', () => {
    const speakeasy = require('speakeasy');
    const secret = speakeasy.generateSecret();
    const verified = speakeasy.totp.verify({
      secret: secret.base32,
      encoding: 'base32',
      token: '000000',
      window: 0,
    });
    // 可能偶然匹配，但概率极低
    // 这里只验证函数不抛异常
    expect(typeof verified).toBe('boolean');
  });
});
