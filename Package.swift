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
        .package(path: "packages/webex-question-core"),
        .package(url: "https://github.com/SwiftyLab/MetaCodable.git", exact: "1.0.0")
    ],
    targets: [
        .executableTarget(
            name: "GetWebexSpaceMacApp",
            dependencies: [
                .product(name: "WebexQuestionGeneratorCore", package: "webex-question-core"),
                .product(name: "MetaCodable", package: "MetaCodable")
            ],
            path: "apps/cubicle-macos/Sources",
            resources: [
                .process("Resources")
            ]
        ),
        .testTarget(
            name: "GetWebexSpaceMacAppTests",
            dependencies: ["GetWebexSpaceMacApp"],
            path: "apps/cubicle-macos/Tests"
        )
    ]
)
