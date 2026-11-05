import SwiftUI

/// Settings › Webhooks.
struct WebhooksSettings: View {
    var body: some View {
        SettingsWebFallback(page: .webhooks)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .webhooks, id: id)
        }
    }
}
