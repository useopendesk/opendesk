import SwiftUI

/// Settings › Phone.
struct PhoneSettings: View {
    var body: some View {
        SettingsWebFallback(page: .phone)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .phone, id: id)
        }
    }
}
