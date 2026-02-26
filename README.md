# cuh

Computer Use Helpers (**cuh**) is a Go module for automating common user-machine actions.

## Vision

Provide small, focused packages that can be imported by Go scripts (and CPE agents) to perform user-approved actions on a machine, such as:

- sending messages on macOS
- managing Gmail email flows
- using a browser for simple automations

## Initial package layout

- `macos/messages` - helpers for sending messages from macOS.
- `gmail` - helpers for reading/sending/managing Gmail workflows.
- `browser` - helpers for opening and driving browser actions.

## Status

This repository is initialized with API placeholders. Implementations will be added incrementally.
