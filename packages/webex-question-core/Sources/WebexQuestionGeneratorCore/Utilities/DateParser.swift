import Foundation

/// Parses common Webex export timestamp formats.
public struct DateParser: Sendable {
    public init() {}

    public func parse(_ value: String?) -> Date? {
        guard let raw = value?.trimmingCharacters(in: .whitespacesAndNewlines), !raw.isEmpty else {
            return nil
        }
        if let seconds = TimeInterval(raw), seconds > 1_000_000_000 {
            return Date(timeIntervalSince1970: seconds)
        }
        for formatter in Self.isoFormatters {
            if let date = formatter.date(from: raw) { return date }
        }
        for formatter in Self.dateFormatters {
            if let date = formatter.date(from: raw) { return date }
        }
        return nil
    }

    private static let isoFormatters: [ISO8601DateFormatter] = {
        let withFractional = ISO8601DateFormatter()
        withFractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let internet = ISO8601DateFormatter()
        internet.formatOptions = [.withInternetDateTime]
        let dateOnly = ISO8601DateFormatter()
        dateOnly.formatOptions = [.withFullDate]
        return [withFractional, internet, dateOnly]
    }()

    private static let dateFormatters: [DateFormatter] = {
        let formats = [
            "yyyy-MM-dd HH:mm:ss ZZZZZ",
            "yyyy-MM-dd HH:mm:ss zzz",
            "yyyy-MM-dd HH:mm:ss",
            "yyyy-MM-dd HH:mm",
            "yyyy/MM/dd HH:mm:ss",
            "MM/dd/yyyy HH:mm:ss",
            "MM/dd/yyyy HH:mm",
            "M/d/yyyy h:mm a",
            "M/d/yy h:mm a",
            "MMM d, yyyy h:mm a",
            "EEE MMM d HH:mm:ss yyyy"
        ]
        return formats.map { format in
            let formatter = DateFormatter()
            formatter.locale = Locale(identifier: "en_US_POSIX")
            formatter.timeZone = TimeZone(secondsFromGMT: 0)
            formatter.dateFormat = format
            return formatter
        }
    }()
}
