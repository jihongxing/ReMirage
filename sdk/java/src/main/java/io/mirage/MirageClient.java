package io.mirage;

import io.grpc.ManagedChannel;
import io.grpc.ManagedChannelBuilder;
import io.grpc.Metadata;
import io.grpc.stub.MetadataUtils;

public class MirageClient implements AutoCloseable {
    private final ManagedChannel channel;
    private final String token;
    private final GatewayService gatewayService;
    private final BillingService billingService;
    private final CellService cellService;

    private MirageClient(Builder builder) {
        ManagedChannelBuilder<?> channelBuilder = ManagedChannelBuilder
            .forTarget(builder.endpoint);
        
        if (builder.useTls) {
            channelBuilder.useTransportSecurity();
        } else {
            channelBuilder.usePlaintext();
        }
        
        this.channel = channelBuilder.build();
        this.token = builder.token;
        
        Metadata metadata = new Metadata();
        metadata.put(
            Metadata.Key.of("authorization", Metadata.ASCII_STRING_MARSHALLER),
            "Bearer " + token
        );
        
        this.gatewayService = new GatewayService(channel, metadata);
        this.billingService = new BillingService(channel, metadata);
        this.cellService = new CellService(channel, metadata);
    }

    public static Builder builder() {
        return new Builder();
    }

    public GatewayService gateway() {
        return gatewayService;
    }

    public BillingService billing() {
        return billingService;
    }

    public CellService cell() {
        return cellService;
    }

    @Override
    public void close() {
        channel.shutdown();
    }

    public static class Builder {
        private String endpoint;
        private String token;
        private boolean useTls = true;
        private int timeoutSeconds = 30;

        public Builder endpoint(String endpoint) {
            this.endpoint = endpoint;
            return this;
        }

        public Builder token(String token) {
            this.token = token;
            return this;
        }

        public Builder useTls(boolean useTls) {
            this.useTls = useTls;
            return this;
        }

        public Builder timeout(int seconds) {
            this.timeoutSeconds = seconds;
            return this;
        }

        public MirageClient build() {
            return new MirageClient(this);
        }
    }
}
