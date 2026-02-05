## 2025-05-23 - [Accessibility Consistency in Icon Buttons]

**Learning:** The application uses several icon-only buttons (Send, Search, Settings, Stop, etc.) which lacked descriptive ARIA labels, making them inaccessible to screen readers. Additionally, tooltips were used inconsistently between standard 'title' attributes and Radix UI Tooltip components.
**Action:** Always ensure icon-only buttons have an 'aria-label' and prefer the Radix UI 'Tooltip' component over the 'title' attribute for visual hints to maintain UI consistency and accessibility.
