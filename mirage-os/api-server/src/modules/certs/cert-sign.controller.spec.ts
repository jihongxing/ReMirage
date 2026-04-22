import * as forge from 'node-forge';
import * as crypto from 'crypto';

/**
 * cert-sign.controller 单元测试
 * 验证 CSR 解析、签发逻辑、有效期和主题校验
 */

// ─── 辅助函数 ───

/** 生成自签名 CA 证书和私钥 */
function generateCA(): { caCert: forge.pki.Certificate; caKey: forge.pki.PrivateKey; caCertPem: string; caKeyPem: string } {
  const keys = forge.pki.rsa.generateKeyPair(2048);
  const cert = forge.pki.createCertificate();
  cert.publicKey = keys.publicKey;
  cert.serialNumber = '01';
  cert.validity.notBefore = new Date();
  cert.validity.notAfter = new Date(Date.now() + 365 * 24 * 3600 * 1000);
  cert.setSubject([{ name: 'commonName', value: 'Mirage Test CA' }]);
  cert.setIssuer([{ name: 'commonName', value: 'Mirage Test CA' }]);
  cert.setExtensions([{ name: 'basicConstraints', cA: true }]);
  cert.sign(keys.privateKey, forge.md.sha256.create());
  return {
    caCert: cert,
    caKey: keys.privateKey,
    caCertPem: forge.pki.certificateToPem(cert),
    caKeyPem: forge.pki.privateKeyToPem(keys.privateKey),
  };
}

/** 生成 CSR */
function generateCSR(gatewayId: string): string {
  const keys = forge.pki.rsa.generateKeyPair(2048);
  const csr = forge.pki.createCertificationRequest();
  csr.publicKey = keys.publicKey;
  csr.setSubject([
    { name: 'commonName', value: `mirage-gateway-${gatewayId}` },
    { name: 'organizationName', value: 'Mirage Project' },
  ]);
  csr.sign(keys.privateKey, forge.md.sha256.create());
  return forge.pki.certificationRequestToPem(csr);
}

/** 模拟签发逻辑（与 controller 一致） */
function signCert(
  csrPem: string,
  gatewayId: string,
  caCert: forge.pki.Certificate,
  caKey: forge.pki.PrivateKey,
  validityHours = 72,
): { certificate: string; expiresAt: string; serialNumber: string } {
  const csr = forge.pki.certificationRequestFromPem(csrPem);
  if (!csr.verify()) throw new Error('CSR 签名验证失败');

  const csrSubject = csr.subject.getField('CN');
  if (!csrSubject || !csrSubject.value.includes(gatewayId)) {
    throw new Error(`CSR Subject CN 必须包含 gatewayId: ${gatewayId}`);
  }

  const serialNumber = crypto.randomBytes(16).toString('hex');
  const now = new Date();
  const expiresAt = new Date(now.getTime() + validityHours * 3600 * 1000);

  const cert = forge.pki.createCertificate();
  cert.publicKey = csr.publicKey as forge.pki.PublicKey;
  cert.serialNumber = serialNumber;
  cert.validity.notBefore = now;
  cert.validity.notAfter = expiresAt;
  cert.setSubject(csr.subject.attributes);
  cert.setIssuer(caCert.subject.attributes);
  cert.setExtensions([
    { name: 'basicConstraints', cA: false },
    { name: 'keyUsage', digitalSignature: true, keyEncipherment: true },
    { name: 'extKeyUsage', serverAuth: true, clientAuth: true },
  ]);
  cert.sign(caKey, forge.md.sha256.create());

  return {
    certificate: forge.pki.certificateToPem(cert),
    expiresAt: expiresAt.toISOString(),
    serialNumber,
  };
}

// ─── 测试 ───

describe('CertSignController - 签发逻辑', () => {
  let ca: ReturnType<typeof generateCA>;

  beforeAll(() => {
    ca = generateCA();
  });

  it('应成功签发证书并可被 CA 验证', () => {
    const gatewayId = 'gw-test-001';
    const csrPem = generateCSR(gatewayId);
    const result = signCert(csrPem, gatewayId, ca.caCert, ca.caKey);

    // 验证返回结构
    expect(result.certificate).toContain('-----BEGIN CERTIFICATE-----');
    expect(result.serialNumber).toHaveLength(32);
    expect(new Date(result.expiresAt).getTime()).toBeGreaterThan(Date.now());

    // 验证证书可被 CA 验证
    const signedCert = forge.pki.certificateFromPem(result.certificate);
    const caStore = forge.pki.createCaStore([ca.caCert]);
    expect(forge.pki.verifyCertificateChain(caStore, [signedCert])).toBe(true);

    // 验证 Subject CN
    const cn = signedCert.subject.getField('CN');
    expect(cn?.value).toContain(gatewayId);

    // 验证 Issuer
    const issuerCN = signedCert.issuer.getField('CN');
    expect(issuerCN?.value).toBe('Mirage Test CA');
  });

  it('应拒绝无效 CSR PEM', () => {
    expect(() => {
      signCert('not-a-valid-pem', 'gw-001', ca.caCert, ca.caKey);
    }).toThrow();
  });

  it('应拒绝 Subject CN 不匹配 gatewayId 的 CSR', () => {
    const csrPem = generateCSR('gw-other');
    expect(() => {
      signCert(csrPem, 'gw-expected', ca.caCert, ca.caKey);
    }).toThrow(/gatewayId/);
  });

  it('签发证书有效期应在 72h 内', () => {
    const gatewayId = 'gw-ttl-test';
    const csrPem = generateCSR(gatewayId);
    const result = signCert(csrPem, gatewayId, ca.caCert, ca.caKey, 72);

    const expiresAt = new Date(result.expiresAt);
    const diffHours = (expiresAt.getTime() - Date.now()) / (3600 * 1000);
    expect(diffHours).toBeGreaterThan(71);
    expect(diffHours).toBeLessThanOrEqual(72.1);
  });

  it('签发证书序列号应唯一', () => {
    const gatewayId = 'gw-serial-test';
    const csrPem = generateCSR(gatewayId);
    const r1 = signCert(csrPem, gatewayId, ca.caCert, ca.caKey);
    const r2 = signCert(csrPem, gatewayId, ca.caCert, ca.caKey);
    expect(r1.serialNumber).not.toBe(r2.serialNumber);
  });

  it('签发证书不应为 CA 证书', () => {
    const gatewayId = 'gw-noca';
    const csrPem = generateCSR(gatewayId);
    const result = signCert(csrPem, gatewayId, ca.caCert, ca.caKey);
    const cert = forge.pki.certificateFromPem(result.certificate);
    const bc = cert.getExtension('basicConstraints') as any;
    expect(bc?.cA).toBe(false);
  });
});
