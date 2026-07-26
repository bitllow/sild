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

    /// A conversation target opens that thread directly (single-thread mode: back
    /// closes rather than returning to a list that was never shown).
    private var rootIsHome: Bool {
        if case .conversation = target { return false }
        return true
    }

    public var body: some View {
        let style = SildStyle(config: model.state.brand, systemDark: colorScheme == .dark)
        Group {
            if !rootIsHome {
                ThreadScreen(
                    model: model, state: model.state, draft: false,
                    onBack: onClose, onClose: onClose
                )
            } else if model.draft || model.state.activeId != nil {
                // ONE ThreadScreen for both draft and created states so the composer keeps
                // its text and attachments across the transition — the first send creates
                // the conversation while the user's draft is still in the field.
                ThreadScreen(
                    model: model, state: model.state, draft: model.draft,
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
        .preferredColorScheme(model.state.brand.theme == "auto" ? nil : (model.state.brand.theme == "dark" ? .dark : .light))
        .onAppear {
            if case let .conversation(id) = target {
                model.start(conversationId: id)
            } else {
                model.start()
            }
        }
        .onDisappear { model.close() }
    }
}
