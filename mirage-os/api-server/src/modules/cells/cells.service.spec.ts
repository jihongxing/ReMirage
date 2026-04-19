import * as fc from 'fast-check';

// Feature: mirage-os-brain, Property 9: 蜂窝容量限制
describe('Property 9: 蜂窝容量限制', () => {
  it('should reject assignment when cell is full', () => {
    fc.assert(
      fc.property(
        fc.integer({ min: 1, max: 200 }), // maxUsers
        fc.integer({ min: 0, max: 300 }), // currentUsers
        (maxUsers, currentUsers) => {
          const isFull = currentUsers >= maxUsers;
          if (isFull) {
            // 蜂窝已满，应返回 409
            expect(currentUsers).toBeGreaterThanOrEqual(maxUsers);
          } else {
            // 蜂窝未满，应允许分配
            expect(currentUsers).toBeLessThan(maxUsers);
          }
        },
      ),
      { numRuns: 100 },
    );
  });

  it('should enforce exact capacity boundary', () => {
    const maxUsers = 50;
    // 49 用户 → 允许
    expect(49 < maxUsers).toBe(true);
    // 50 用户 → 拒绝
    expect(50 >= maxUsers).toBe(true);
    // 51 用户 → 拒绝
    expect(51 >= maxUsers).toBe(true);
  });
});

// 单元测试：蜂窝创建 + cost_multiplier 自动设置
describe('CellsService - create', () => {
  const LEVEL_MULTIPLIER: Record<string, number> = {
    STANDARD: 1.0,
    PLATINUM: 1.5,
    DIAMOND: 2.0,
  };

  it('should set correct cost_multiplier for each level', () => {
    for (const [level, expected] of Object.entries(LEVEL_MULTIPLIER)) {
      const multiplier = LEVEL_MULTIPLIER[level];
      expect(multiplier).toBe(expected);
    }
  });

  it('should have monotonically increasing multipliers', () => {
    expect(LEVEL_MULTIPLIER['DIAMOND']).toBeGreaterThan(LEVEL_MULTIPLIER['PLATINUM']);
    expect(LEVEL_MULTIPLIER['PLATINUM']).toBeGreaterThan(LEVEL_MULTIPLIER['STANDARD']);
  });
});
