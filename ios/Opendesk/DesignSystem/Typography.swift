import CoreText
import SwiftUI
import UIKit

/// Geist and Geist Mono, bundled (SIL OFL, `Resources/Fonts`), registered
/// at launch. Falls back to the system font if a face is missing, so the app
/// never renders without text.
enum ODFont {
    private static let faces = [
        "Geist-Regular", "Geist-Medium", "Geist-SemiBold", "Geist-Bold",
        "GeistMono-Regular", "GeistMono-Medium",
    ]

    /// Registers the bundled faces for this process. Safe to call more than once.
    static func register() {
        for face in faces where UIFont(name: face, size: 12) == nil {
            let url = Bundle.main.url(forResource: face, withExtension: "ttf")
                ?? Bundle.main.url(forResource: face, withExtension: "ttf", subdirectory: "Fonts")
            if let url {
                CTFontManagerRegisterFontsForURL(url as CFURL, .process, nil)
            }
        }
    }

    private static func name(for weight: Font.Weight, mono: Bool) -> String {
        if mono { return weight == .medium || weight == .semibold || weight == .bold ? "GeistMono-Medium" : "GeistMono-Regular" }
        switch weight {
        case .bold, .heavy, .black: return "Geist-Bold"
        case .semibold: return "Geist-SemiBold"
        case .medium: return "Geist-Medium"
        default: return "Geist-Regular"
        }
    }

    /// Geist at `size`, scaling with Dynamic Type relative to `style`.
    static func sans(_ size: CGFloat, weight: Font.Weight = .regular, relativeTo style: Font.TextStyle = .body) -> Font {
        let face = name(for: weight, mono: false)
        guard UIFont(name: face, size: size) != nil else { return .system(size: size, weight: weight) }
        return .custom(face, size: size, relativeTo: style)
    }

    /// Geist Mono for numbers, ids and timers.
    static func mono(_ size: CGFloat, weight: Font.Weight = .regular, relativeTo style: Font.TextStyle = .caption) -> Font {
        let face = name(for: weight, mono: true)
        guard UIFont(name: face, size: size) != nil else { return .system(size: size, weight: weight, design: .monospaced) }
        return .custom(face, size: size, relativeTo: style)
    }

    // The scale (dense and calm).
    static var largeTitle: Font { sans(28, weight: .semibold, relativeTo: .largeTitle) }
    static var title: Font { sans(20, weight: .semibold, relativeTo: .title3) }
    static var headline: Font { sans(16, weight: .semibold, relativeTo: .headline) }
    static var body: Font { sans(15.5, relativeTo: .body) }
    static var bodyMedium: Font { sans(15.5, weight: .medium, relativeTo: .body) }
    static var callout: Font { sans(14, relativeTo: .callout) }
    static var subheadline: Font { sans(13.5, relativeTo: .subheadline) }
    static var footnote: Font { sans(12.5, relativeTo: .footnote) }
    static var caption: Font { sans(11.5, weight: .medium, relativeTo: .caption) }
}

extension View {
    /// Tight tracking for headings, as the web's `tracking-tight`.
    func headingTracking(_ size: CGFloat = 20) -> some View {
        tracking(-size * 0.02)
    }
}
