-- CreateTable
CREATE TABLE "audit_logs" (
    "id" TEXT NOT NULL,
    "operator_id" TEXT NOT NULL,
    "operator_role" TEXT NOT NULL,
    "source_ip" TEXT NOT NULL,
    "target_resource" TEXT NOT NULL,
    "action_type" TEXT NOT NULL,
    "action_params" JSONB,
    "result" TEXT NOT NULL,
    "created_at" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "audit_logs_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "audit_logs_created_at_idx" ON "audit_logs"("created_at");

-- CreateIndex
CREATE INDEX "audit_logs_operator_id_idx" ON "audit_logs"("operator_id");

-- CreateIndex
CREATE INDEX "audit_logs_action_type_idx" ON "audit_logs"("action_type");
