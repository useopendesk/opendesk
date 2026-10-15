import SwiftUI

// MARK: - Avatars

struct Avatar: View {
    let name: String
    var url: String?
    var seed: String = ""
    var size: CGFloat = 28
    /// A ring in the presence tone (replying amber, viewing green).
    var ring: Color?

    var body: some View {
        let tone = Theme.avatarTone(for: seed.isEmpty ? name : seed)
        ZStack {
            Circle().fill(tone.soft)
            Circle().strokeBorder(tone.line, lineWidth: 1)
            Text(Format.initials(name))
                .font(ODFont.sans(size * 0.38, weight: .semibold))
                .foregroundStyle(tone.text)
            if let url, let resolved = URL(string: url), resolved.scheme == "https" || resolved.scheme == "http" {
                AsyncImage(url: resolved, transaction: Transaction(animation: Motion.fade)) { phase in
                    if case let .success(image) = phase {
                        image.resizable().scaledToFill().transition(.opacity)
                    }
                }
                .clipShape(Circle())
            }
        }
        .frame(width: size, height: size)
        .overlay {
            if let ring {
                Circle().strokeBorder(ring, lineWidth: 2).padding(-3)
            }
        }
        .accessibilityHidden(true)
    }
}

/// Overlapping avatars of people on a ticket right now.
struct PresenceStack: View {
    let viewers: [PresenceViewer]
    var size: CGFloat = 22
    var max = 3

    var body: some View {
        HStack(spacing: -size * 0.3) {
            ForEach(viewers.prefix(max)) { viewer in
                Avatar(name: viewer.name, url: viewer.avatar, seed: viewer.userID, size: size)
                    .padding(1.5)
                    .background(Circle().fill(viewer.state == .replying ? Tone.amber.solid : Tone.green.solid))
                    .transition(.scale.combined(with: .opacity))
            }
            if viewers.count > max {
                Text("+\(viewers.count - max)")
                    .font(ODFont.mono(10, weight: .medium))
                    .foregroundStyle(Theme.fg2)
                    .frame(width: size + 3, height: size + 3)
                    .background(Theme.active, in: Circle())
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(viewers.map { "\($0.name) is \($0.state == .replying ? "replying" : "viewing")" }.joined(separator: ", "))
    }
}

// MARK: - Panels and sections

/// A white panel on the grey chrome, with a hairline.
struct Panel<Content: View>: View {
    var padding: CGFloat = 0
    @ViewBuilder var content: () -> Content

    var body: some View {
        VStack(alignment: .leading, spacing: 0) { content() }
            .padding(padding)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: Theme.Radius.card, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
    }
}

struct SectionLabel: View {
    let text: String
    init(_ text: String) { self.text = text }

    var body: some View {
        Text(text)
            .font(ODFont.sans(12.5, weight: .medium))
            .foregroundStyle(Theme.fg3)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct Hairline: View {
    var inset: CGFloat = 0

    var body: some View {
        Rectangle().fill(Theme.lineSoft).frame(height: 1).padding(.leading, inset)
    }
}

/// A row in a settings-style panel.
struct PanelRow<Trailing: View>: View {
    let title: String
    var subtitle: String?
    var symbol: String?
    var tint: Color = Theme.fg
    @ViewBuilder var trailing: () -> Trailing

    var body: some View {
        HStack(spacing: 12) {
            if let symbol {
                Image(systemName: symbol)
                    .font(.system(size: 15, weight: .regular))
                    .foregroundStyle(tint == Theme.fg ? Theme.fg2 : tint)
                    .frame(width: 22)
            }
            VStack(alignment: .leading, spacing: 2) {
                Text(title).font(ODFont.sans(15)).foregroundStyle(tint)
                if let subtitle {
                    Text(subtitle).font(ODFont.footnote).foregroundStyle(Theme.fg3)
                }
            }
            Spacer(minLength: 8)
            trailing()
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 12)
        .frame(minHeight: 48)
        .contentShape(Rectangle())
    }
}

extension PanelRow where Trailing == EmptyView {
    init(title: String, subtitle: String? = nil, symbol: String? = nil, tint: Color = Theme.fg) {
        self.init(title: title, subtitle: subtitle, symbol: symbol, tint: tint, trailing: { EmptyView() })
    }
}

/// The disclosure chevron for navigation rows.
struct Chevron: View {
    var body: some View {
        Image(systemName: "chevron.right").font(.system(size: 12, weight: .semibold)).foregroundStyle(Theme.fg4)
    }
}

// MARK: - States

/// The shared empty state: a mark, a title, one line of help, and an optional action.
struct EmptyState<Mark: View>: View {
    let title: String
    var message: String?
    var actionTitle: String?
    var action: (() -> Void)?
    @ViewBuilder var mark: () -> Mark

    var body: some View {
        VStack(spacing: 14) {
            mark()
                .frame(width: 56, height: 56)
                .background(Theme.surface, in: RoundedRectangle(cornerRadius: 16, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: 16, style: .continuous).strokeBorder(Theme.line, lineWidth: 1))
            VStack(spacing: 6) {
                Text(title)
                    .font(ODFont.sans(17, weight: .semibold))
                    .foregroundStyle(Theme.fg)
                    .multilineTextAlignment(.center)
                if let message {
                    Text(message)
                        .font(ODFont.callout)
                        .foregroundStyle(Theme.fg3)
                        .multilineTextAlignment(.center)
                        .frame(maxWidth: 300)
                }
            }
            if let actionTitle, let action {
                Button(actionTitle, action: action)
                    .buttonStyle(OutlineButtonStyle(fullWidth: false, height: 38))
                    .padding(.top, 4)
            }
        }
        .padding(32)
        .frame(maxWidth: .infinity)
    }
}

extension EmptyState where Mark == EmptyStateSymbol {
    init(symbol: String, title: String, message: String? = nil, actionTitle: String? = nil, action: (() -> Void)? = nil) {
        self.init(title: title, message: message, actionTitle: actionTitle, action: action) { EmptyStateSymbol(symbol: symbol) }
    }
}

struct EmptyStateSymbol: View {
    let symbol: String

    var body: some View {
        Image(systemName: symbol).font(.system(size: 22, weight: .regular)).foregroundStyle(Theme.fg3)
    }
}

/// A failed load with a retry.
struct ErrorState: View {
    var title = "Couldn't load this"
    let message: String
    let retry: () -> Void

    var body: some View {
        EmptyState(symbol: "wifi.exclamationmark", title: title, message: message, actionTitle: "Try again", action: retry)
    }
}

/// Grey placeholders shaped like ticket rows while the first page loads.
struct SkeletonRows: View {
    var count = 7
    @State private var dim = false
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        VStack(spacing: 0) {
            ForEach(0 ..< count, id: \.self) { index in
                HStack(alignment: .top, spacing: 12) {
                    Circle().fill(Theme.active).frame(width: 14, height: 14).padding(.top, 2)
                    VStack(alignment: .leading, spacing: 8) {
                        RoundedRectangle(cornerRadius: 4).fill(Theme.active).frame(width: [120, 150, 96, 170][index % 4], height: 11)
                        RoundedRectangle(cornerRadius: 4).fill(Theme.hover).frame(height: 11).padding(.trailing, [40, 80, 20, 60][index % 4])
                    }
                    Spacer()
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 14)
                Hairline(inset: 42)
            }
        }
        .opacity(dim ? 0.55 : 1)
        .onAppear {
            guard !reduceMotion else { return }
            withAnimation(.easeInOut(duration: 0.9).repeatForever()) { dim = true }
        }
        .accessibilityLabel("Loading")
    }
}

/// A soft notice line inside a panel (info, warning).
struct Notice: View {
    let text: String
    var tone: Tone = .gray
    var symbol = "info.circle"

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Image(systemName: symbol).font(.system(size: 13))
            Text(text).font(ODFont.footnote).fixedSize(horizontal: false, vertical: true)
        }
        .foregroundStyle(tone == .gray ? Theme.fg2 : tone.text)
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(tone == .gray ? Theme.sunken : tone.soft, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous).strokeBorder(tone == .gray ? Theme.line : tone.line, lineWidth: 1))
    }
}

// MARK: - Inputs

/// A labelled text field on a white panel with a hairline.
struct ODTextField: View {
    let label: String
    @Binding var text: String
    var prompt = ""
    var secure = false
    var contentType: UITextContentType?
    var keyboard: UIKeyboardType = .default
    var submitLabel: SubmitLabel = .next
    var onSubmit: () -> Void = {}

    @FocusState private var focused: Bool
    @State private var revealed = false

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(label).font(ODFont.sans(13, weight: .medium)).foregroundStyle(Theme.fg2)
            HStack(spacing: 8) {
                Group {
                    if secure && !revealed {
                        SecureField(text: $text) { Text(prompt).foregroundStyle(Theme.fg4) }
                    } else {
                        TextField(text: $text) { Text(prompt).foregroundStyle(Theme.fg4) }
                    }
                }
                .font(ODFont.sans(16))
                .foregroundStyle(Theme.fg)
                .textContentType(contentType)
                .keyboardType(keyboard)
                .textInputAutocapitalization(keyboard == .emailAddress || keyboard == .URL || secure ? .never : .sentences)
                .autocorrectionDisabled(keyboard == .emailAddress || keyboard == .URL || secure)
                .submitLabel(submitLabel)
                .focused($focused)
                .onSubmit(onSubmit)
                if secure {
                    Button {
                        revealed.toggle()
                    } label: {
                        Image(systemName: revealed ? "eye.slash" : "eye").font(.system(size: 14)).foregroundStyle(Theme.fg3)
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(revealed ? "Hide password" : "Show password")
                }
            }
            .padding(.horizontal, 13)
            .frame(height: 46)
            .background(Theme.surface, in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous)
                    .strokeBorder(focused ? Theme.lineStrong : Theme.line, lineWidth: focused ? 1.5 : 1)
            )
            .motion(Motion.fade, value: focused)
        }
    }
}

/// An inline error line under a form.
struct FormError: View {
    let text: String?

    var body: some View {
        if let text, !text.isEmpty {
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                Image(systemName: "exclamationmark.circle.fill").font(.system(size: 12))
                Text(text).font(ODFont.footnote).fixedSize(horizontal: false, vertical: true)
            }
            .foregroundStyle(Tone.red.text)
            .frame(maxWidth: .infinity, alignment: .leading)
            .transition(.opacity.combined(with: .move(edge: .top)))
        }
    }
}

// MARK: - Toasts

struct Toast: Identifiable, Equatable {
    enum Kind: Equatable { case info, success, error }

    let id = UUID()
    var message: String
    var kind: Kind = .info
    var actionTitle: String?
    var action: (() -> Void)?

    static func == (lhs: Toast, rhs: Toast) -> Bool { lhs.id == rhs.id }
}

@MainActor
@Observable
final class ToastCenter {
    private(set) var current: Toast?
    @ObservationIgnored private var dismissTask: Task<Void, Never>?

    func show(_ toast: Toast, seconds: Double = 3.2) {
        current = toast
        dismissTask?.cancel()
        dismissTask = Task { [weak self] in
            try? await Task.sleep(for: .seconds(seconds))
            guard !Task.isCancelled else { return }
            self?.current = nil
        }
    }

    func show(_ message: String, kind: Toast.Kind = .info) {
        show(Toast(message: message, kind: kind))
    }

    func error(_ error: Error) {
        guard !error.isCancellation else { return }
        Haptics.error()
        show(Toast(message: error.userMessage, kind: .error), seconds: 4.5)
    }

    func dismiss() {
        current = nil
    }
}

struct ToastView: View {
    let toast: Toast
    let dismiss: () -> Void

    var body: some View {
        HStack(spacing: 10) {
            switch toast.kind {
            case .success: Image(systemName: "checkmark.circle.fill").foregroundStyle(Tone.green.solid)
            case .error: Image(systemName: "exclamationmark.circle.fill").foregroundStyle(Tone.red.solid)
            case .info: EmptyView()
            }
            Text(toast.message)
                .font(ODFont.sans(14, weight: .medium))
                .foregroundStyle(Theme.fg)
                .lineLimit(2)
            Spacer(minLength: 4)
            if let title = toast.actionTitle, let action = toast.action {
                Button(title) {
                    action()
                    dismiss()
                }
                .font(ODFont.sans(14, weight: .semibold))
                .foregroundStyle(Theme.fg)
            }
        }
        .padding(.horizontal, 16)
        .frame(minHeight: 48)
        .glassEffect(.regular, in: RoundedRectangle(cornerRadius: 16, style: .continuous))
        .padding(.horizontal, 16)
        .onTapGesture(perform: dismiss)
        .accessibilityAddTraits(.isStaticText)
    }
}
