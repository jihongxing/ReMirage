import { Controller, Post, Body, UseGuards, BadRequestException, InternalServerErrorException, Logger } from '@nestjs/common';
import { InternalHMACGuard } from '../../common/internal-hmac.guard';
import * as crypto from 'crypto';
import * as forge from 'node-forge';
import * as fs from 'fs';
import * as path from 'path';

interface CertSignRequest {
  csr: string;        // PEM 编码的 CSR
  gatewayId: string;  // 网关标识
}

interface CertSignResponse {
  certificate: string; // PEM 编码的叶子证书
  expiresAt: string;   // ISO 8601 过期时间
  serialNumber: string;
}

/**
 * 证书签发 API - POST /internal/cert/sign
 * 接收 Gateway CSR，签发 24h~72h 短期叶子证书
 * Guard: InternalHMACGuard + mTLS
 */
@Controller('internal/cert')
@UseGuards(InternalHMACGuard)
export class CertSignController {
  private readonly logger = new Logger(CertSignController.name);
  private caCert: forge.pki.Certificate | null = null;
  private caKey: forge.pki.PrivateKey | null = null;

  constructor() {
    this.loadCA();
  }

  /** 从环境变量指定路径加载 CA 证书和私钥 */
  private loadCA(): void {
    const caDir = process.env.CA_DIR || '/etc/mirage/certs';
    const caCertPath = path.join(caDir, 'ca.crt');
    const caKeyPath = path.join(caDir, 'ca.key');

    try {
      const certPem = fs.readFileSync(caCertPath, 'utf-8');
      const keyPem = fs.readFileSync(caKeyPath, 'utf-8');
      this.caCert = forge.pki.certificateFromPem(certPem);
      this.caKey = forge.pki.privateKeyFromPem(keyPem);
      this.logger.log(`CA 证书已加载: ${caCertPath}`);
    } catch (err) {
      this.logger.warn(`CA 证书加载失败（签发功能不可用）: ${err.message}`);
    }
  }

  @Post('sign')
  async signCert(@Body() body: CertSignRequest): Promise<CertSignResponse> {
    if (!body.csr || !body.gatewayId) {
      throw new BadRequestException('csr 和 gatewayId 为必填项');
    }

    if (!this.caCert || !this.caKey) {
      throw new InternalServerErrorException('CA 证书未加载，无法签发');
    }

    // 1. 解析 CSR
    let csr: forge.pki.CertificateSigningRequest;
    try {
      csr = forge.pki.certificationRequestFromPem(body.csr);
    } catch {
      throw new BadRequestException('CSR 格式无效，无法解析 PEM');
    }

    // 2. 验证 CSR 签名
    if (!csr.verify()) {
      throw new BadRequestException('CSR 签名验证失败');
    }

    // 3. 验证 Subject CN 包含 gatewayId
    const csrSubject = csr.subject.getField('CN');
    if (!csrSubject || !csrSubject.value.includes(body.gatewayId)) {
      throw new BadRequestException(`CSR Subject CN 必须包含 gatewayId: ${body.gatewayId}`);
    }

    // 4. 有效期配置（24h~72h）
    const validityHours = parseInt(process.env.CERT_VALIDITY_HOURS || '72', 10);
    if (validityHours < 24 || validityHours > 72) {
      throw new BadRequestException('证书有效期必须在 24h~72h 之间');
    }

    // 5. 生成序列号
    const serialNumber = crypto.randomBytes(16).toString('hex');

    // 6. 签发证书
    const now = new Date(Date.now() - 60_000);
    const expiresAt = new Date(now.getTime() + validityHours * 3600 * 1000);

    const cert = forge.pki.createCertificate();
    cert.version = 2;
    cert.publicKey = csr.publicKey as forge.pki.PublicKey;
    cert.serialNumber = serialNumber;
    cert.validity.notBefore = now;
    cert.validity.notAfter = expiresAt;

    // Subject 从 CSR 继承
    cert.setSubject(csr.subject.attributes);

    // Issuer 从 CA 证书继承
    cert.setIssuer(this.caCert.subject.attributes);

    // 扩展
    cert.setExtensions([
      { name: 'basicConstraints', cA: false },
      {
        name: 'keyUsage',
        digitalSignature: true,
        keyEncipherment: true,
      },
      {
        name: 'extKeyUsage',
        serverAuth: true,
        clientAuth: true,
      },
      {
        name: 'subjectAltName',
        altNames: [
          { type: 2, value: 'mirage-gateway' },       // DNS
          { type: 2, value: 'localhost' },             // DNS
          { type: 7, ip: '127.0.0.1' },               // IP
        ],
      },
      {
        name: 'authorityKeyIdentifier',
        keyIdentifier: true,
        authorityCertIssuer: true,
        serialNumber: true,
      },
    ]);

    // 使用 CA 私钥签名
    cert.sign(this.caKey as forge.pki.rsa.PrivateKey, forge.md.sha256.create());

    const certPem = forge.pki.certificateToPem(cert);

    this.logger.log(`证书已签发: gateway=${body.gatewayId}, serial=${serialNumber}, expires=${expiresAt.toISOString()}`);

    return {
      certificate: certPem,
      expiresAt: expiresAt.toISOString(),
      serialNumber,
    };
  }
}
