// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "CubicleMonorepo",
    platforms: [
        .macOS(.v13)
    ],
    products: [
        .executable(name: "Cubicle", targets: ["GetWebexSpaceMacApp"])
    ],
    dependencies: [
        .package(path: "packages/webex-question-core")
    ],
    targets: [
        .executableTarget(
            name: "GetWebexSpaceMacApp",
            dependencies: [
                .product(name: "WebexQuestionGeneratorCore", package: "webex-question-core")
            ],
            path: "apps/cubicle-macos/Sources"
        ),
        .testTarget(
            name: "GetWebexSpaceMacAppTests",
            dependencies: ["GetWebexSpaceMacApp"],
            path: "apps/cubicle-macos/Tests"
        )
    ]
)

