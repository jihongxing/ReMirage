// swift-tools-version:5.7
import PackageDescription

let package = Package(
    name: "MirageSDK",
    platforms: [.macOS(.v12), .iOS(.v15)],
    products: [
        .library(name: "MirageSDK", targets: ["MirageSDK"]),
    ],
    dependencies: [
        .package(url: "https://github.com/grpc/grpc-swift.git", from: "1.19.0"),
    ],
    targets: [
        .target(
            name: "MirageSDK",
            dependencies: [
                .product(name: "GRPC", package: "grpc-swift"),
            ]
        ),
    ]
)
