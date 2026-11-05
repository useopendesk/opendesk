import SwiftUI

/// Settings › AI agent › Tests.
struct TestsSettings: View {
    var body: some View {
        SettingsWebFallback(page: .tests)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .tests, id: id)
        }
    }
}
