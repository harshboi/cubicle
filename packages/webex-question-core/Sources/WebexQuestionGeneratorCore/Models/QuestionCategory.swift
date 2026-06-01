import Foundation

/// Analytical question families supported by the deterministic generator.
public enum QuestionCategory: String, Codable, Sendable, CaseIterable, Hashable {
    case descriptive
    case diagnostic
    case comparative
    case behavioral
    case network
    case efficiency
    case predictive
}
