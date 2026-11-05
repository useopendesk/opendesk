import SwiftUI

/// Settings › AI agent › Behaviour.
struct BehaviourSettings: View {
    var body: some View {
        SettingsWebFallback(page: .behaviour)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .behaviour, id: id)
        }
    }
}
