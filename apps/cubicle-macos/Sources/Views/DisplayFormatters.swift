import Foundation
import SwiftUI

/// Shared display/date/link formatting helpers for SwiftUI views.
enum DisplayFormatters {
    static func latestMessageLabel(_ rawValue: String) -> String {
        guard !rawValue.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return "Latest Message: unknown"
        }
        return "Latest Message: \(localDateTime(rawValue))"
    }

    static func snapshotUpdatedLabel(_ rawValue: String) -> String {
        guard !rawValue.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return "Webex Checked: unknown"
        }
        return "Webex Checked: \(localDateTime(rawValue))"
    }

    static func localDateTime(_ rawValue: String) -> String {
        guard let date = parseDate(rawValue) else {
            return rawValue
        }
        return localDateTimeFormatter.string(from: date)
    }

    static func readableDetailLine(_ line: String) -> String {
        let prefixes = [
            ("Live Webex Sync:", "Webex Checked:"),
            ("Latest room message:", "Latest Message:"),
            ("Space cache updated at:", "Space Cache Updated:"),
            ("Person cache updated at:", "Person Cache Updated:"),
            ("Summary generated:", "Analysis Generated:"),
            ("Last started:", "Last Started:"),
            ("Last completed:", "Last Completed:")
        ]
        let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
        for (sourcePrefix, displayPrefix) in prefixes where trimmed.hasPrefix(sourcePrefix) {
            let value = trimmed.dropFirst(sourcePrefix.count)
                .trimmingCharacters(in: .whitespacesAndNewlines)
            guard !value.isEmpty else {
                return line
            }
            return "\(displayPrefix) \(localDateTime(value))"
        }
        if let repliedRange = trimmed.range(of: " last replied:") {
            let subject = trimmed[..<repliedRange.lowerBound]
            let value = trimmed[repliedRange.upperBound...]
                .trimmingCharacters(in: .whitespacesAndNewlines)
            guard !subject.isEmpty, !value.isEmpty else {
                return replaceInlineISOTimestamps(in: line)
            }
            return "\(subject) Last Replied: \(localDateTime(value))"
        }
        return replaceInlineISOTimestamps(in: line)
    }

    static func linkifiedText(
        _ text: String,
        boldPrefixThrough delimiter: String? = nil,
        boldSoWhat: Bool = false
    ) -> AttributedString {
        var attributed = AttributedString(text)
        if let delimiter,
           let range = attributed.range(of: delimiter) {
            attributed[attributed.startIndex..<range.upperBound].font = .body.bold()
        }
        if boldSoWhat,
           let range = attributed.range(of: "So what:") {
            attributed[range].font = .body.bold()
        }

        applyLinks(in: text, to: &attributed)
        return attributed
    }

    private static func applyLinks(in text: String, to attributed: inout AttributedString) {
        applyMatches(
            pattern: #"(?:https?|webexteams)://[^\s<>"']+"#,
            in: text,
            to: &attributed
        ) { URL(string: $0) }

        applyMatches(
            pattern: #"(?:/Volumes|/Users|/private|/tmp)/[^\s,;)\]]+"#,
            in: text,
            to: &attributed
        ) { URL(fileURLWithPath: $0) }
    }

    private static func applyMatches(
        pattern: String,
        in text: String,
        to attributed: inout AttributedString,
        makeURL: (String) -> URL?
    ) {
        guard let regex = try? NSRegularExpression(pattern: pattern) else {
            return
        }
        let nsRange = NSRange(text.startIndex..<text.endIndex, in: text)
        for match in regex.matches(in: text, range: nsRange) {
            guard var stringRange = Range(match.range, in: text) else {
                continue
            }
            trimTrailingLinkPunctuation(text: text, range: &stringRange)
            let value = String(text[stringRange])
            guard let url = makeURL(value),
                  let attributedRange = Range(stringRange, in: attributed) else {
                continue
            }
            attributed[attributedRange].link = url
            attributed[attributedRange].foregroundColor = .blue
        }
    }

    private static func trimTrailingLinkPunctuation(text: String, range: inout Range<String.Index>) {
        let trailing = CharacterSet(charactersIn: ".,;:)]}\"'")
        while range.lowerBound < range.upperBound {
            let lastIndex = text.index(before: range.upperBound)
            let scalar = text[lastIndex].unicodeScalars.first
            guard let scalar, trailing.contains(scalar) else {
                break
            }
            range = range.lowerBound..<lastIndex
        }
    }

    private static func replaceInlineISOTimestamps(in text: String) -> String {
        let pattern = #"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})"#
        guard let regex = try? NSRegularExpression(pattern: pattern) else {
            return text
        }
        let nsRange = NSRange(text.startIndex..<text.endIndex, in: text)
        let matches = regex.matches(in: text, range: nsRange).reversed()
        var result = text
        for match in matches {
            guard let range = Range(match.range, in: result) else {
                continue
            }
            let rawDate = String(result[range])
            guard parseDate(rawDate) != nil else {
                continue
            }
            result.replaceSubrange(range, with: localDateTime(rawDate))
        }
        return result
    }

    static func parseDate(_ rawValue: String) -> Date? {
        let value = rawValue.trimmingCharacters(in: .whitespacesAndNewlines)
        if let date = iso8601WithFractionalSeconds.date(from: value) {
            return date
        }
        if let date = iso8601.date(from: value) {
            return date
        }
        if let date = legacyLocalSecondFormatter.date(from: value) {
            return date
        }
        return legacyLocalMinuteFormatter.date(from: value)
    }

    private static let localDateTimeFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = .current
        formatter.dateFormat = "MM/dd/yyyy HH:mm:ss z"
        return formatter
    }()

    private static let iso8601WithFractionalSeconds: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    private static let iso8601: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter
    }()

    private static let legacyLocalSecondFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateFormat = "yyyy-MM-dd HH:mm:ss z"
        return formatter
    }()

    private static let legacyLocalMinuteFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateFormat = "yyyy-MM-dd HH:mm z"
        return formatter
    }()
}
