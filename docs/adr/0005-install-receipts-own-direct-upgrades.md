# Install receipts own direct upgrades

Skillet's first-party installer records an Install receipt after a verified
direct installation and requires that receipt before treating a later binary
as its own. PATH inspection alone cannot reliably distinguish a direct Skillet
install from a Homebrew-managed or unrelated executable; the small durable
record makes same-channel Upgrade auditable and lets collisions fail safely.

The installer may recover a missing receipt only through an explicit adoption
or replacement action. It never silently claims an existing binary, and it
never uses its receipt to modify an installation owned by Homebrew.
