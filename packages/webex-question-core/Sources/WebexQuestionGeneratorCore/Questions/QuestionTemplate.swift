import Foundation

/// Lightweight template for deterministic generated-question text.
public struct QuestionTemplate: Codable, Sendable, Hashable {
    public var category: QuestionCategory
    public var template: String
    public var suggestedAnalysis: String

    public init(category: QuestionCategory, template: String, suggestedAnalysis: String) {
        self.category = category
        self.template = template
        self.suggestedAnalysis = suggestedAnalysis
    }

    public func render(_ values: [String: String]) -> String {
        values.reduce(template) { partial, entry in
            partial.replacingOccurrences(of: "{\(entry.key)}", with: entry.value)
        }
    }
}
