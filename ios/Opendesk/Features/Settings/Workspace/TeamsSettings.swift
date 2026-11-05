import SwiftUI

/// Settings › Teams.
struct TeamsSettings: View {
    var body: some View {
        SettingsWebFallback(page: .teams)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .teams, id: id)
        }
    }
}
