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
