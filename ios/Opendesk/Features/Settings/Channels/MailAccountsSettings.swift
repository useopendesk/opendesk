import SwiftUI

/// Settings › Mail accounts.
struct MailAccountsSettings: View {
    var body: some View {
        SettingsWebFallback(page: .mailAccounts)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .mailAccounts, id: id)
        }
    }
}
