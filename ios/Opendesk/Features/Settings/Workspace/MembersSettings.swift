import SwiftUI

/// Settings › Members.
struct MembersSettings: View {
    var body: some View {
        SettingsWebFallback(page: .members)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .members, id: id)
        }
    }
}
