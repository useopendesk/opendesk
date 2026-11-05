import SwiftUI

/// Settings › Mailboxes.
struct MailboxesSettings: View {
    var body: some View {
        SettingsWebFallback(page: .mailboxes)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .mailboxes, id: id)
        }
    }
}
