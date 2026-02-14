"""Document extraction modules.

All extractors produce a list of Block objects in document order.
Block is the shared data structure across all extractors.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Literal

BlockType = Literal["heading", "paragraph", "table", "list", "code"]


@dataclass
class Block:
    """A structured block extracted from a document."""

    type: BlockType
    text: str
    meta: dict = field(default_factory=dict)
