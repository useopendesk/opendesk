import SwiftUI

/// Settings › Routing.
struct RoutingSettings: View {
    var body: some View {
        SettingsWebFallback(page: .routing)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .routing, id: id)
        }
    }
}
