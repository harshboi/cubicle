import Foundation

public enum Statistics {
    public static func mean(_ values: [Double]) -> Double? {
        guard !values.isEmpty else { return nil }
        return values.reduce(0, +) / Double(values.count)
    }

    public static func median(_ values: [Double]) -> Double? {
        guard !values.isEmpty else { return nil }
        let sorted = values.sorted()
        let middle = sorted.count / 2
        if sorted.count.isMultiple(of: 2) {
            return (sorted[middle - 1] + sorted[middle]) / 2
        }
        return sorted[middle]
    }

    public static func percentile(_ values: [Double], _ p: Double) -> Double? {
        guard !values.isEmpty else { return nil }
        let sorted = values.sorted()
        let clamped = max(0, min(1, p))
        let position = clamped * Double(sorted.count - 1)
        let lower = Int(floor(position))
        let upper = Int(ceil(position))
        if lower == upper { return sorted[lower] }
        let weight = position - Double(lower)
        return sorted[lower] * (1 - weight) + sorted[upper] * weight
    }

    public static func iqr(_ values: [Double]) -> Double? {
        guard let q1 = percentile(values, 0.25), let q3 = percentile(values, 0.75) else { return nil }
        return q3 - q1
    }

    public static func upperOutlierThreshold(_ values: [Double], multiplier: Double = 1.5) -> Double? {
        guard let q3 = percentile(values, 0.75), let spread = iqr(values) else { return nil }
        return q3 + multiplier * spread
    }

    public static func lowerOutlierThreshold(_ values: [Double], multiplier: Double = 1.5) -> Double? {
        guard let q1 = percentile(values, 0.25), let spread = iqr(values) else { return nil }
        return q1 - multiplier * spread
    }

    public static func zScore(value: Double, values: [Double]) -> Double? {
        guard let average = mean(values), values.count > 1 else { return nil }
        let variance = values.map { pow($0 - average, 2) }.reduce(0, +) / Double(values.count - 1)
        let standardDeviation = sqrt(variance)
        guard standardDeviation > 0 else { return nil }
        return (value - average) / standardDeviation
    }

    public static func pearson(_ pairs: [(Double, Double)]) -> Double? {
        guard pairs.count > 1 else { return nil }
        let xs = pairs.map(\.0)
        let ys = pairs.map(\.1)
        guard let meanX = mean(xs), let meanY = mean(ys) else { return nil }
        let numerator = pairs.map { ($0.0 - meanX) * ($0.1 - meanY) }.reduce(0, +)
        let denomX = sqrt(xs.map { pow($0 - meanX, 2) }.reduce(0, +))
        let denomY = sqrt(ys.map { pow($0 - meanY, 2) }.reduce(0, +))
        guard denomX > 0, denomY > 0 else { return nil }
        return numerator / (denomX * denomY)
    }
}
