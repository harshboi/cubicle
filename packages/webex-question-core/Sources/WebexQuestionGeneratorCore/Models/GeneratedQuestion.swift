import Foundation

/// A ranked analytical question generated from local metrics and patterns.
public struct GeneratedQuestion: Identifiable, Codable, Sendable, Hashable {
    public let id: UUID
    public var text: String
    public var category: QuestionCategory
    public var rationale: String
    public var supportingMetrics: [String: String]
    public var suggestedAnalysis: String
    public var interestingnessScore: Double
    public var actionabilityScore: Double
    public var confidenceScore: Double
    public var finalScore: Double
    public var relatedDimensions: [String]
    public var relatedMetrics: [String]

    public init(
        id: UUID = UUID(),
        text: String,
        category: QuestionCategory,
        rationale: String,
        supportingMetrics: [String: String] = [:],
        suggestedAnalysis: String,
        interestingnessScore: Double,
        actionabilityScore: Double,
        confidenceScore: Double,
        finalScore: Double = 0,
        relatedDimensions: [String] = [],
        relatedMetrics: [String] = []
    ) {
        self.id = id
        self.text = text
        self.category = category
        self.rationale = rationale
        self.supportingMetrics = supportingMetrics
        self.suggestedAnalysis = suggestedAnalysis
        self.interestingnessScore = interestingnessScore
        self.actionabilityScore = actionabilityScore
        self.confidenceScore = confidenceScore
        self.finalScore = finalScore
        self.relatedDimensions = relatedDimensions
        self.relatedMetrics = relatedMetrics
    }
}
