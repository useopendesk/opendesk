import SwiftUI

/// The primary action: ink fill, white label, slightly square.
struct InkButtonStyle: ButtonStyle {
    var fullWidth = true
    var height: CGFloat = 46
    @Environment(\.isEnabled) private var isEnabled

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(ODFont.sans(15.5, weight: .semibold))
            .foregroundStyle(Theme.inverseFg)
            .frame(maxWidth: fullWidth ? .infinity : nil)
            .frame(height: height)
            .padding(.horizontal, fullWidth ? 0 : 16)
            .background(Theme.inverse.opacity(isEnabled ? 1 : 0.35), in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
            .scaleEffect(configuration.isPressed ? 0.98 : 1)
            .opacity(configuration.isPressed ? 0.88 : 1)
            .animation(Motion.press, value: configuration.isPressed)
    }
}

/// Secondary: white panel with a hairline.
struct OutlineButtonStyle: ButtonStyle {
    var fullWidth = true
    var height: CGFloat = 46
    var tint: Color = Theme.fg

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(ODFont.sans(15.5, weight: .medium))
            .foregroundStyle(tint)
            .frame(maxWidth: fullWidth ? .infinity : nil)
            .frame(height: height)
            .padding(.horizontal, fullWidth ? 0 : 14)
            .background(configuration.isPressed ? Theme.hover : Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
            .scaleEffect(configuration.isPressed ? 0.98 : 1)
            .animation(Motion.press, value: configuration.isPressed)
    }
}

/// A small chip-like button for inline actions (Accept, Skip, Insert).
struct ChipButtonStyle: ButtonStyle {
    var tone: Tone? = nil
    var filled = false

    func makeBody(configuration: Configuration) -> some View {
        let tint = tone?.text ?? Theme.fg
        configuration.label
            .font(ODFont.sans(13, weight: .medium))
            .foregroundStyle(filled ? Theme.inverseFg : tint)
            .padding(.horizontal, 10)
            .frame(height: 30)
            .background {
                RoundedRectangle(cornerRadius: Theme.Radius.chip + 1, style: .continuous)
                    .fill(filled ? Theme.inverse : (tone?.soft ?? Theme.sunken))
            }
            .overlay {
                if !filled {
                    RoundedRectangle(cornerRadius: Theme.Radius.chip + 1, style: .continuous)
                        .strokeBorder(tone?.line ?? Theme.line, lineWidth: 1)
                }
            }
            .opacity(configuration.isPressed ? 0.75 : 1)
            .animation(Motion.press, value: configuration.isPressed)
    }
}

/// Scale-on-press for custom tappable rows and tiles.
struct TapScaleStyle: ButtonStyle {
    var scale: CGFloat = 0.97

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .scaleEffect(configuration.isPressed ? scale : 1)
            .animation(Motion.press, value: configuration.isPressed)
    }
}

/// A row that highlights while pressed, for list rows inside panels.
struct RowPressStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .background(configuration.isPressed ? Theme.hover : Color.clear)
            .contentShape(Rectangle())
    }
}

/// A 44pt Liquid Glass circle with one symbol.
struct GlassCircleButton: View {
    let symbol: String
    let label: String
    var tint: Color = Theme.fg
    var badge = 0
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: symbol)
                .font(.system(size: 16, weight: .medium))
                .foregroundStyle(tint)
                .frame(width: 44, height: 44)
                .contentShape(Circle())
                .overlay(alignment: .topTrailing) {
                    if badge > 0 {
                        Circle().fill(Tone.blue.solid).frame(width: 9, height: 9)
                            .overlay(Circle().strokeBorder(Theme.surface, lineWidth: 1.5))
                            .offset(x: -9, y: 9)
                    }
                }
        }
        .buttonStyle(.plain)
        .glassEffect(.regular.interactive(), in: Circle())
        .accessibilityLabel(label)
    }
}

/// The screen's primary action in a header: an ink circle with one symbol.
struct HeaderPrimaryButton: View {
    let symbol: String
    let label: String
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: symbol)
                .font(.system(size: 17, weight: .semibold))
                .foregroundStyle(Theme.inverseFg)
                .frame(width: 44, height: 44)
                .background(Theme.inverse, in: Circle())
                .contentShape(Circle())
        }
        .buttonStyle(TapScaleStyle(scale: 0.92))
        .accessibilityLabel(label)
    }
}

/// A progress spinner in a button's place while it works.
struct ButtonLabel: View {
    let title: String
    var busy = false
    var symbol: String?

    var body: some View {
        ZStack {
            HStack(spacing: 7) {
                if let symbol { Image(systemName: symbol).font(.system(size: 14, weight: .semibold)) }
                Text(title)
            }
            .opacity(busy ? 0 : 1)
            if busy { ProgressView().tint(Theme.inverseFg).controlSize(.small) }
        }
    }
}
