import Foundation

/// Any JSON value. Used for the open-ended members of the API (`custom`,
/// `fields`, notification and activity `data`, event payloads) and for the
/// Phoenix wire format, so everything that crosses actors stays `Sendable`.
enum JSONValue: Codable, Hashable, Sendable {
    case null
    case bool(Bool)
    case number(Double)
    case string(String)
    case array([JSONValue])
    case object([String: JSONValue])

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        if container.decodeNil() {
            self = .null
        } else if let value = try? container.decode(Bool.self) {
            self = .bool(value)
        } else if let value = try? container.decode(Double.self) {
            self = .number(value)
        } else if let value = try? container.decode(String.self) {
            self = .string(value)
        } else if let value = try? container.decode([JSONValue].self) {
            self = .array(value)
        } else if let value = try? container.decode([String: JSONValue].self) {
            self = .object(value)
        } else {
            throw DecodingError.dataCorruptedError(in: container, debugDescription: "Unsupported JSON value")
        }
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .null: try container.encodeNil()
        case let .bool(value): try container.encode(value)
        case let .number(value): try container.encode(value)
        case let .string(value): try container.encode(value)
        case let .array(value): try container.encode(value)
        case let .object(value): try container.encode(value)
        }
    }

    subscript(key: String) -> JSONValue? {
        if case let .object(object) = self { return object[key] }
        return nil
    }

    subscript(index: Int) -> JSONValue? {
        if case let .array(array) = self, array.indices.contains(index) { return array[index] }
        return nil
    }

    var string: String? {
        if case let .string(value) = self { return value }
        return nil
    }

    var double: Double? {
        if case let .number(value) = self { return value }
        return nil
    }

    var int: Int? {
        if case let .number(value) = self, value.isFinite { return Int(value) }
        return nil
    }

    var bool: Bool? {
        if case let .bool(value) = self { return value }
        return nil
    }

    var array: [JSONValue]? {
        if case let .array(value) = self { return value }
        return nil
    }

    var object: [String: JSONValue]? {
        if case let .object(value) = self { return value }
        return nil
    }

    var isNull: Bool {
        if case .null = self { return true }
        return false
    }

    /// Decodes this value as a model, through the API's coders.
    func decode<T: Decodable>(_ type: T.Type) throws -> T {
        let data = try APICoding.encoder.encode(self)
        return try APICoding.decoder.decode(T.self, from: data)
    }

    /// Parses raw JSON bytes.
    static func parse(_ data: Data) throws -> JSONValue {
        try APICoding.decoder.decode(JSONValue.self, from: data)
    }

    /// Builds a value from any Encodable (a request body for the socket).
    static func from<T: Encodable>(_ value: T) throws -> JSONValue {
        try parse(APICoding.encoder.encode(value))
    }

    /// Plain text for display: strings as-is, numbers without a trailing ".0".
    var displayText: String? {
        switch self {
        case let .string(value): return value
        case let .number(value):
            if value.rounded() == value, abs(value) < 1e15 { return String(Int(value)) }
            return String(value)
        case let .bool(value): return value ? "Yes" : "No"
        case let .array(values):
            let parts = values.compactMap(\.displayText)
            return parts.isEmpty ? nil : parts.joined(separator: ", ")
        default: return nil
        }
    }
}

/// The API's JSON coders: RFC 3339 dates with or without fractional seconds.
enum APICoding {
    static let decoder: JSONDecoder = {
        let decoder = JSONDecoder()
        let fractional = Date.ISO8601FormatStyle(includingFractionalSeconds: true)
        let plain = Date.ISO8601FormatStyle()
        decoder.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let raw = try container.decode(String.self)
            if let date = (try? fractional.parse(raw)) ?? (try? plain.parse(raw)) { return date }
            throw DecodingError.dataCorruptedError(in: container, debugDescription: "Unparseable date: \(raw)")
        }
        return decoder
    }()

    static let encoder: JSONEncoder = {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .custom { date, encoder in
            var container = encoder.singleValueContainer()
            try container.encode(date.formatted(Date.ISO8601FormatStyle(includingFractionalSeconds: false)))
        }
        encoder.outputFormatting = [.withoutEscapingSlashes]
        return encoder
    }()
}

/// Enums from the API that must never fail a whole screen when the server
/// grows a new case: unknown raw values decode as `fallback`.
protocol LenientEnum: RawRepresentable, Codable, Sendable where RawValue == String {
    static var fallback: Self { get }
}

extension LenientEnum {
    init(from decoder: Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(String.self)
        self = Self(rawValue: raw) ?? Self.fallback
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        try container.encode(rawValue)
    }
}

extension KeyedDecodingContainer {
    /// `decodeIfPresent` that also swallows a value of the wrong shape.
    func lenient<T: Decodable>(_ type: T.Type, forKey key: Key) -> T? {
        (try? decodeIfPresent(type, forKey: key)) ?? nil
    }
}
