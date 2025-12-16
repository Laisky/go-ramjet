# GPTChat Frontend Notes

This document consolidates the key requirements, behaviors, and past fixes for the GPTChat web UI so future changes stay aligned.

## Conversation Lifecycle

- Each turn is identified by a `chatID`; user and assistant messages share the same `chatID` and are stored separately under role-specific keys.
- Editing or retrying a user message must **keep its position**: update the user message, reset only its paired assistant message, and stream the replacement reply in place. Do not append a new turn.
- Regenerating an assistant reply also reuses the same `chatID`, clears only the old assistant entry, and streams the new reply into the existing slot. Never append.

## Scrolling Behavior

- Sending a new prompt: enable auto-follow and scroll to the latest reply; continue following during streaming **unless** the user scrolls manually.
- Manual scroll: immediately disables auto-follow to leave control with the user. Returning near the bottom or using the scroll-to-bottom button re-enables it.
- Regenerate: **no automatic scroll**; viewport must remain stable when regenerate is clicked.
- Scroll-to-bottom button: always forces a scroll to the latest content.

## Loading History

- The "Load older messages" control must live at the top of the chat list (above the messages), not mid-thread.
- When older messages are prepended, keep the viewport anchored by compensating scroll position.

## UI Controls

- Edit & Retry: available on user messages; opens an edit modal; retry replaces the paired assistant message in place.
- Regenerate: available on assistant messages; refreshes the assistant content in place without scrolling.
- Voice, MCP, Draw toggles remain functional and independent of the behaviors above.

## Storage and Persistence

- History is stored per session using `chat_user_session_<sessionId>` and message payloads under `chat_data_<role>_<chatID>`.
- Loading reconstructs messages from stored user/assistant pairs and sorts by `chatID` (timestamp-ordered).

## Common Pitfalls (avoid regressions)

- Do **not** delete conversation segments when editing or regenerating; only the targeted assistant needs replacement.
- Do **not** auto-scroll on regenerate; only on send (unless user intervenes).
- Keep the "Load older messages" button at the top; avoid placing it mid-thread.
- Ensure manual scroll cancels auto-follow during streaming.

## Testing Checklist

- Edit a mid-thread user message: the updated assistant reply stays in place; later messages remain.
- Regenerate an assistant reply: content refreshes in place; viewport does not jump.
- Send a new prompt: auto-scroll follows streaming; manual scroll cancels follow.
- Load older messages: control appears at top; viewport remains anchored after loading.
