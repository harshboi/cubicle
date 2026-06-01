// swift-tools-version: 5.9

import PackageDescription

let package = Package(
    name: "WebexQuestionGeneratorCore",
    platforms: [
        .macOS(.v13)
    ],
    products: [
        .library(name: "WebexQuestionGeneratorCore", targets: ["WebexQuestionGeneratorCore"]),
        .executable(name: "WebexQGSmoke", targets: ["WebexQGSmoke"])
    ],
    targets: [
        .target(
            name: "WebexQuestionGeneratorCore",
            path: "Sources/WebexQuestionGeneratorCore"
        ),
        .executableTarget(
            name: "WebexQGSmoke",
            dependencies: ["WebexQuestionGeneratorCore"],
            path: "Examples/WebexQGSmoke"
        ),
        .testTarget(
            name: "WebexQuestionGeneratorCoreTests",
            dependencies: ["WebexQuestionGeneratorCore"],
            path: "Tests/WebexQuestionGeneratorCoreTests"
        )
    ]
)
