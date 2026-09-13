# Branch recovery note

On 2026-09-13 the product naming edits (`Manifold Nexus` → `Manifold Agent`) were initially applied on an older branch head and temporarily hid the later MNL-005 core implementation commits from the feature branch tip.

The implementation lineage rooted at commit `f3456541efe6262da511a072369b706bbd7078ea` contains the MNL-005 persistence sources, template/compiler core, structured evaluation boundary, deterministic policy core and implementation status document.

This recovery line preserves that implementation and applies the Manifold Agent naming/product-boundary work on top. The canonical feature branch must include this lineage before further MNL application-service work proceeds.
