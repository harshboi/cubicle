import Foundation

public enum TextUtilities {
    public static let urlPattern = #"https?://[^\s]+"#
    public static let emailPattern = #"[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}"#

    public static func collapseWhitespace(_ text: String) -> String {
        text.components(separatedBy: .whitespacesAndNewlines)
            .filter { !$0.isEmpty }
            .joined(separator: " ")
    }

    public static func words(in text: String) -> [String] {
        tokenize(text).filter { $0.count > 1 }
    }

    public static func tokenize(_ text: String) -> [String] {
        text.lowercased().split { character in
            !character.isLetter && !character.isNumber && character != "'"
        }.map(String.init)
    }

    public static func extractMentions(from text: String) -> [String] {
        var mentions: [String] = []
        let tokens = text.split(whereSeparator: { $0.isWhitespace })
        for token in tokens {
            let trimmed = token.trimmingCharacters(in: CharacterSet(charactersIn: ",.!?:;()[]{}<>\"'"))
            if trimmed.hasPrefix("@"), trimmed.count > 1 {
                mentions.append(String(trimmed.dropFirst()))
            } else if trimmed.range(of: emailPattern, options: [.regularExpression, .caseInsensitive]) != nil {
                mentions.append(trimmed.lowercased())
            }
        }
        return Array(Set(mentions)).sorted()
    }

    public static func redactURLs(_ text: String) -> String {
        text.replacingOccurrences(of: urlPattern, with: "[URL]", options: .regularExpression)
    }

    public static func redactEmails(_ text: String) -> String {
        text.replacingOccurrences(of: emailPattern, with: "[EMAIL]", options: [.regularExpression, .caseInsensitive])
    }

    public static func isQuestion(_ text: String) -> Bool {
        let lowered = text.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard !lowered.isEmpty else { return false }
        if lowered.contains("?") { return true }
        let questionStarts = ["who", "what", "when", "where", "why", "how", "can", "could", "should", "would", "is", "are", "do", "does", "did"]
        return questionStarts.contains { lowered.hasPrefix($0 + " ") }
    }
}
