import SwiftUI

/// Settings › Business hours.
struct BusinessHoursSettings: View {
    var body: some View {
        SettingsWebFallback(page: .businessHours)
    }

    struct Detail: View {
        let id: String

        var body: some View {
            SettingsWebFallback(page: .businessHours, id: id)
        }
    }
}
