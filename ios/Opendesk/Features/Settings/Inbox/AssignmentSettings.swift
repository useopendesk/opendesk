import SwiftUI

/// Settings › Assignment.
struct AssignmentSettings: View {
    var body: some View {
        SettingsWebFallback(page: .assignment)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .assignment, id: id)
        }
    }
}
