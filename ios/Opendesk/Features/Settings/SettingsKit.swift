import SwiftUI

// Pieces the workspace settings pages share, on top of the Account kit (AccountPage,
// AccountSection, AccountRow, AccountToggleRow, AccountSheetHeader, AccountConfirm).

/// A row on the Account tab's home that opens a settings page, or for a page that stays on
/// the web, that page in the in-app browser.
struct SettingsLinkRow: View {
    let page: SettingsPage
    var value: String?
    var subtitle: String?

    @Environment(AppEnvironment.self) private var env
    @State private var web: AccountWebPage?

    var body: some View {
        if page.webOnly {
            Button {
                Haptics.tap()
                web = env.session.instance.webURL(path: page.webPath).map(AccountWebPage.init)
            } label: {
                AccountRow(symbol: page.symbol, tone: page.tone, title: page.title, subtitle: subtitle) {
                    Image(systemName: "arrow.up.right")
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundStyle(Theme.fg4)
                }
            }
            .buttonStyle(RowPressStyle())
            .accountWebSheet($web)
        } else {
            AccountLink(route: .setting(page)) {
                AccountRow(symbol: page.symbol, tone: page.tone, title: page.title, subtitle: subtitle) {
                    AccountDisclosure(value: value)
                }
            }
        }
    }
}

/// Opens something a settings page lists (a team, a mailbox, a webhook) on the stack above it.
/// `id` names it to the page's own `Detail`; a nested one joins ids with "/" ("brand/widget").
struct SettingsDetailLink<Label: View>: View {
    let page: SettingsPage
    let id: String
    @ViewBuilder var label: () -> Label

    @Environment(AppEnvironment.self) private var env

    var body: some View {
        Button {
            env.router.accountPath.append(.detail(page, id))
        } label: {
            label()
        }
        .buttonStyle(RowPressStyle())
    }
}

extension Router {
    /// Opens a settings page's detail from code, after creating one.
    func openSetting(_ page: SettingsPage, id: String) {
        accountPath.append(.detail(page, id))
    }
}

/// Unsaved changes on a settings page: what is waiting, Discard and Save, on glass where the tab
/// bar floats (`settingsSaveBar`).
struct SettingsSaveBar: View {
    var title = "Unsaved changes"
    /// Why it can't save yet, in place of the title.
    var problem: String?
    let saving: Bool
    var canSave = true
    let save: () -> Void
    let discard: () -> Void

    private var shape: RoundedRectangle { RoundedRectangle(cornerRadius: 28, style: .continuous) }

    var body: some View {
        HStack(spacing: 10) {
            Circle()
                .fill(problem == nil ? Tone.amber.solid : Tone.red.solid)
                .frame(width: 7, height: 7)
                .accessibilityHidden(true)
            Text(problem ?? title)
                .font(ODFont.sans(13.5, weight: .medium))
                .foregroundStyle(problem == nil ? Theme.fg2 : Tone.red.text)
                .lineLimit(2)
                .fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 4)
            Button("Discard", action: discard)
                .buttonStyle(OutlineButtonStyle(fullWidth: false, height: 40))
                .disabled(saving)
            Button(action: save) { ButtonLabel(title: "Save", busy: saving) }
                .buttonStyle(InkButtonStyle(fullWidth: false, height: 40))
                .disabled(!canSave || saving)
        }
        .padding(.leading, 18)
        .padding(.trailing, 8)
        .frame(minHeight: 56)
        .background(Theme.surface.opacity(0.9), in: shape)
        .glassEffect(.regular, in: shape)
        .accessibilityElement(children: .contain)
        .accessibilityLabel("Unsaved changes")
    }
}

private struct SettingsSaveBarModifier: ViewModifier {
    let dirty: Bool
    let guarded: Bool
    let bar: SettingsSaveBar

    @Environment(\.dismiss) private var dismiss
    @Environment(AppEnvironment.self) private var env
    @State private var confirm: AccountConfirm?
    @State private var leaving = false

    func body(content: Content) -> some View {
        content
            .safeAreaInset(edge: .bottom, spacing: 0) {
                if dirty {
                    bar
                        .padding(.horizontal, 16)
                        .padding(.bottom, 4)
                        .transition(.move(edge: .bottom).combined(with: .opacity))
                }
            }
            .motion(Motion.snappy, value: dirty)
            .tabBarHidden(dirty)
            .navigationBarBackButtonHidden(guarded)
            .toolbar {
                if guarded {
                    ToolbarItem(placement: .topBarLeading) {
                        Button {
                            Haptics.tap()
                            askToLeave()
                        } label: {
                            Image(systemName: "chevron.left")
                        }
                        .accessibilityLabel("Back")
                    }
                }
            }
            .sheet(item: $confirm, onDismiss: {
                guard leaving else { return }
                leaving = false
                dismiss()
            }) { confirm in AccountConfirmSheet(confirm: confirm) }
            #if DEBUG
            // The QA harness's `/demo/settings-back`: the back button on a page with changes.
            .onChange(of: env.router.demoAction) { _, action in
                guard action == "settings-back", guarded else { return }
                env.router.demoAction = nil
                askToLeave()
            }
            #endif
    }

    private func askToLeave() {
        confirm = AccountConfirm(
            title: "Discard your changes?",
            message: "What you changed on this page isn't saved yet.",
            confirmTitle: "Discard"
        ) {
            bar.discard()
            leaving = true
        }
    }
}

extension View {
    /// While `dirty`, the save bar takes the tab bar's place under the page, and going back asks
    /// before throwing the changes away. `guarded` turns that question off for a page whose draft
    /// outlives it.
    func settingsSaveBar(
        dirty: Bool,
        guarded: Bool? = nil,
        title: String = "Unsaved changes",
        problem: String? = nil,
        saving: Bool,
        canSave: Bool = true,
        save: @escaping () -> Void,
        discard: @escaping () -> Void
    ) -> some View {
        modifier(SettingsSaveBarModifier(
            dirty: dirty,
            guarded: guarded ?? dirty,
            bar: SettingsSaveBar(title: title, problem: problem, saving: saving, canSave: canSave, save: save, discard: discard)
        ))
    }
}

/// A note at the top of a page a member can read but only admins change.
struct SettingsReadOnlyNote: View {
    var text = "Only admins can change these settings."

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: "lock")
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(Theme.fg3)
            Text(text)
                .font(ODFont.footnote)
                .foregroundStyle(Theme.fg2)
                .fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 11)
        .background(Theme.active.opacity(0.6), in: RoundedRectangle(cornerRadius: Theme.Radius.control, style: .continuous))
    }
}

/// A settings page that lives on the web app (`SettingsPage.webOnly`): what it is for, and
/// the web app's page in the in-app browser.
struct SettingsWebFallback: View {
    let page: SettingsPage
    var id: String?

    @Environment(AppEnvironment.self) private var env
    @State private var web: AccountWebPage?

    var body: some View {
        AccountPage(title: page.title) {
            EmptyState(
                symbol: page.symbol,
                title: "\(page.title) is in the web app",
                message: "It opens the web app here. The web app keeps its own sign-in and may ask you to sign in once.",
                actionTitle: "Open in the web app"
            ) { open() }
            .frame(maxWidth: .infinity)
            .padding(.top, 60)
        }
        .accountWebSheet($web)
    }

    private func open() {
        let path = id.map { "\(page.webPath)/\($0)" } ?? page.webPath
        web = env.session.instance.webURL(path: path).map(AccountWebPage.init)
    }
}
