import SwiftUI

/// Settings › AI clients.
struct AIClientsSettings: View {
    var body: some View {
        SettingsWebFallback(page: .aiClients)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .aiClients, id: id)
        }
    }
}
