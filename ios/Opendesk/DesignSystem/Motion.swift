import SwiftUI
import UIKit

/// Subtle and fast, shared presets (web `lib/motion.ts`). Every animation goes
/// through `Motion.animation(_:reduced:)` or checks Reduce Motion itself.
enum Motion {
    static let press = Animation.spring(response: 0.22, dampingFraction: 0.75)
    static let snappy = Animation.spring(response: 0.32, dampingFraction: 0.86)
    static let smooth = Animation.spring(response: 0.42, dampingFraction: 0.88)
    static let fade = Animation.easeOut(duration: 0.16)

    /// The animation, or none when Reduce Motion is on.
    static func animation(_ animation: Animation, reduced: Bool) -> Animation? {
        reduced ? nil : animation
    }
}

extension View {
    /// `withAnimation`-free animation that honours Reduce Motion.
    func motion<V: Equatable>(_ animation: Animation, value: V) -> some View {
        modifier(ReducedMotionAnimation(animation: animation, value: value))
    }
}

private struct ReducedMotionAnimation<V: Equatable>: ViewModifier {
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    let animation: Animation
    let value: V

    func body(content: Content) -> some View {
        content.animation(reduceMotion ? nil : animation, value: value)
    }
}

/// A one-shot highlight wash when a row changes live (a realtime pulse).
struct LivePulse: ViewModifier {
    let trigger: Date
    @State private var flash = false
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    func body(content: Content) -> some View {
        content
            .background(Tone.blue.soft.opacity(flash ? 1 : 0))
            .onChange(of: trigger) { _, _ in
                guard !reduceMotion else { return }
                flash = true
                withAnimation(.easeOut(duration: 1.1).delay(0.15)) { flash = false }
            }
    }
}

extension View {
    func livePulse(_ trigger: Date) -> some View { modifier(LivePulse(trigger: trigger)) }
}

/// Haptics, quiet and few.
@MainActor
enum Haptics {
    static func tap() { UIImpactFeedbackGenerator(style: .light).impactOccurred() }
    static func success() { UINotificationFeedbackGenerator().notificationOccurred(.success) }
    static func warning() { UINotificationFeedbackGenerator().notificationOccurred(.warning) }
    static func error() { UINotificationFeedbackGenerator().notificationOccurred(.error) }
    static func selection() { UISelectionFeedbackGenerator().selectionChanged() }
}
