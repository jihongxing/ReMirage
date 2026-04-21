-- CreateTable: gateway_sessions
CREATE TABLE "gateway_sessions" (
    "id" TEXT NOT NULL,
    "session_id" TEXT NOT NULL,
    "gateway_id" TEXT NOT NULL,
    "user_id" TEXT NOT NULL,
    "client_id" TEXT,
    "status" TEXT NOT NULL DEFAULT 'active',
    "connected_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "disconnected_at" TIMESTAMP(3),
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "gateway_sessions_pkey" PRIMARY KEY ("id")
);

-- CreateTable: client_sessions
CREATE TABLE "client_sessions" (
    "id" TEXT NOT NULL,
    "session_id" TEXT NOT NULL,
    "client_id" TEXT NOT NULL,
    "user_id" TEXT NOT NULL,
    "current_gateway_id" TEXT,
    "status" TEXT NOT NULL DEFAULT 'active',
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "client_sessions_pkey" PRIMARY KEY ("id")
);

-- AlterTable: billing_logs — add session_id and sequence_number
ALTER TABLE "billing_logs" ADD COLUMN "session_id" TEXT;
ALTER TABLE "billing_logs" ADD COLUMN "sequence_number" BIGINT;

-- CreateIndex: gateway_sessions
CREATE UNIQUE INDEX "gateway_sessions_session_id_key" ON "gateway_sessions"("session_id");
CREATE INDEX "gateway_sessions_gateway_id_status_idx" ON "gateway_sessions"("gateway_id", "status");
CREATE INDEX "gateway_sessions_user_id_status_idx" ON "gateway_sessions"("user_id", "status");
CREATE INDEX "gateway_sessions_client_id_idx" ON "gateway_sessions"("client_id");

-- CreateIndex: client_sessions
CREATE UNIQUE INDEX "client_sessions_session_id_key" ON "client_sessions"("session_id");
CREATE INDEX "client_sessions_client_id_idx" ON "client_sessions"("client_id");
CREATE INDEX "client_sessions_user_id_status_idx" ON "client_sessions"("user_id", "status");
CREATE INDEX "client_sessions_current_gateway_id_idx" ON "client_sessions"("current_gateway_id");

-- CreateIndex: billing_logs idempotency unique constraint
CREATE UNIQUE INDEX "billing_logs_gateway_id_sequence_number_key" ON "billing_logs"("gateway_id", "sequence_number");

-- AddForeignKey: gateway_sessions → gateways
ALTER TABLE "gateway_sessions" ADD CONSTRAINT "gateway_sessions_gateway_id_fkey" FOREIGN KEY ("gateway_id") REFERENCES "gateways"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey: gateway_sessions → users
ALTER TABLE "gateway_sessions" ADD CONSTRAINT "gateway_sessions_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey: client_sessions → users
ALTER TABLE "client_sessions" ADD CONSTRAINT "client_sessions_user_id_fkey" FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
