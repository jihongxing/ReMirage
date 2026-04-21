-- AlterTable: Add ttl_seconds, expires_at, source fields to threat_intel
ALTER TABLE "threat_intel" ADD COLUMN "ttl_seconds" INTEGER NOT NULL DEFAULT 3600;
ALTER TABLE "threat_intel" ADD COLUMN "expires_at" TIMESTAMP(3);
ALTER TABLE "threat_intel" ADD COLUMN "source" TEXT NOT NULL DEFAULT 'auto';

-- CreateIndex
CREATE INDEX "threat_intel_expires_at_idx" ON "threat_intel"("expires_at");
