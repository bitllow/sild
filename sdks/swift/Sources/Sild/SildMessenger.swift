import SwiftUI
import SildCore

/// Which screen the messenger opens on, mirroring the Android entry points.
public enum SildTarget: Equatable {
    /// The conversation list ("Messages").
    case list
    /// Start (or resume) a support request — lands on Home; the request is created on
    /// the first message.
    case support
    /// Open a specific conversation directly, e.g. the trip's driver chat.
    case conversation(String)
}

/// `fullScreenCover(item:)` and `sheet(item:)` need identity; the target is its own.
extension SildTarget: Identifiable {
    public var id: String {
        switch self {
        case .list: return "list"
        case .support: return "support"
        case let .conversation(id): return "conv:\(id)"
        }
    }
}

/// The SDK's SwiftUI entry point: hand it a `SildConfig` and present it.
///
/// ```swift
/// SildMessenger(config: config, target: .support) { dismiss() }
/// ```
///
/// The view is a pure renderer — every decision (peer derivation, lazy create,
/// reconnect catch-up, author mapping) lives in the shared `SildClient`.
public struct SildMessenger: View {
    @Environment(\.colorScheme) private var colorScheme
    @State private var model: SildModel

    private let target: SildTarget
    private let onClose: () -> Void

    public init(
        config: SildConfig,
        target: SildTarget = .list,
        onChime: @escaping () -> Void = {},
        onClose: @escaping () -> Void = {}
    ) {
        _model = State(initialValue: SildModel(config: config, onChime: onChime))
        self.target = target
        self.onClose = onClose
    }

    /// A conversation target opens that thread directly: back closes rather than
    /// returning to a list that was never shown.
    private var isSingleThread: Bool {
        if case .conversation = target { return true }
        return false
    }

    public var body: some View {
        let style = model.style(systemDark: colorScheme == .dark)
        Group {
            if isSingleThread {
                ThreadScreen(model: model, draft: false, onBack: onClose, onClose: onClose)
            } else if model.draft || model.state.activeId != nil {
                // ONE ThreadScreen for both draft and created states so the composer keeps
                // its text and attachments across the transition — the first send creates
                // the conversation while the user's draft is still in the field.
                ThreadScreen(
                    model: model, draft: model.draft,
                    onBack: { model.backToList() }, onClose: onClose
                )
            } else {
                HomeScreen(
                    state: model.state,
                    onNew: { model.beginDraft() },
                    onOpen: { model.openConversation($0) },
                    onToggleSound: { model.toggleSound() },
                    onClose: onClose
                )
            }
        }
        .environment(\.sildStyle, style)
        // Read through the model, so a published update that moves state.stringsRevision
        // redraws the screens rather than sitting in memory.
        .environment(\.sildStrings, { [model] key, vars in
            _ = model.state.stringsRevision
            return model.t(key, vars)
        })
        .environment(\.sildPlurals, { [model] base, count, vars in
            _ = model.state.stringsRevision
            return model.tPlural(base, Int(count), vars)
        })
        .preferredColorScheme(style.preferredScheme)
        .onAppear {
            if case let .conversation(id) = target {
                model.start(conversationId: id)
            } else {
                model.start()
            }
            // On screen, so it answers whether an arriving nudge is one the user
            // is already looking at (§5.5). A host that only presents the messenger
            // never called Sild.initialize, so its config is the SDK's config.
            if SildHost.shared.config == nil { SildHost.shared.config = model.pushConfig }
            SildHost.shared.onScreen = model.pushClient
            // Re-appearing on screen re-checks a held manifest that has aged out.
            // What it downloads is staged for the next start, never applied here.
            model.onForeground()
        }
        .onDisappear {
            if SildHost.shared.onScreen === model.pushClient { SildHost.shared.onScreen = nil }
            model.close()
        }
    }
}
