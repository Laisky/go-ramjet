## 2025-05-15 - [Chat Performance Optimization]

**Learning:** In a chat application where messages are streamed token by token, the entire message list often re-renders. When messages contain large base64 images, this causes significant UI jank. 'React.memo' on the message component is the most effective way to prevent these redundant re-renders. Additionally, 'decoding="async"' on image tags is crucial for offloading the heavy work of base64 image decoding from the main thread.
**Action:** Always memoize core list components in real-time interfaces and use asynchronous image decoding for heavy assets.

## 2025-05-20 - [Search Filtering Debouncing]

**Learning:** In interfaces with large lists (like chat histories), executing complex fuzzy search filtering on every keystroke can lead to noticeable UI lag, especially on lower-end devices or with long conversations. Debouncing the search logic by even a small amount (200ms) significantly improves the perceived responsiveness of the input field by ensuring the CPU isn't overwhelmed by redundant filtering operations during rapid typing.
**Action:** Always debounce search filtering logic in client-side list filtering components.
