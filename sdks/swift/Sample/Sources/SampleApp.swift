import SwiftUI
import Sild
import SildCore

// The "Acme Rides" demo — the iOS counterpart of the Android :sample: a trip card
// ("Message driver" → the peer conversation) and a support card ("Open support"),
// wired to a dev token provider.
@main
struct SampleApp: App {
    var body: some Scene {
        WindowGroup { RootView() }
    }
}

struct RootView: View {
    @State private var target: SildTarget?
    @State private var driverConversationId: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Acme Rides").font(.system(size: 28, weight: .heavy))

            card(
                title: "Trip in progress",
                subtitle: "Toomas is on the way",
                action: "Message driver",
                identifier: "sample.messageDriver"
            ) {
                Task {
                    driverConversationId = try? await DevBackend.ensureDriverConversation(DevBackend.driverTripRef)
                    if let id = driverConversationId { target = .conversation(id) }
                }
            }

            card(
                title: "Need a hand?",
                subtitle: "We reply in a few minutes",
                action: "Open support",
                identifier: "sample.openSupport"
            ) {
                target = .support
            }

            Spacer()
        }
        .padding(20)
        .frame(maxWidth: .infinity, alignment: .leading)
        .fullScreenCover(item: $target) { t in
            SildMessenger(config: DevBackend.config, target: t) { target = nil }
        }
    }

    private func card(
        title: String,
        subtitle: String,
        action: String,
        identifier: String,
        onTap: @escaping () -> Void
    ) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(title).font(.system(size: 17, weight: .semibold))
            Text(subtitle).font(.system(size: 14)).foregroundStyle(.secondary)
            Button(action: onTap) {
                Text(action)
                    .font(.system(size: 15, weight: .semibold))
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 12)
            }
            .buttonStyle(.borderedProminent)
            .padding(.top, 8)
            .accessibilityIdentifier(identifier)
        }
        .padding(16)
        .background(Color(.secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 14))
    }
}
