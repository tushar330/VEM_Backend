# ROOM MAPPING API CONTRACT

This document serves as the official integration reference for the Room Mapping feature on the frontend.

## 🎯 Important Concepts & Data Relationships

### 1. Families & Guests
- **Guests** are individual people attending the event.
- **Families** are logical groupings of guests who should room together.
- In the database, the `family_id` column groups guests.
- **Frontend Logic**: You must group guests by `family_id` to display them as a single draggable unit.

### 2. Room Inventory & Offers
- **RoomOfferID**: Unique ID for a specific type of room (e.g., "Deluxe King").
- **RoomsInventory**: A JSONB column on the `events` table that tracks availability.
- **Logic**: When a room is allocated (or updated), the backend automatically decrements/increments the `available` count in `rooms_inventory`.
- **Safety**: The backend enforces inventory limits. Attempts to allocate to a room with `available <= 0` will fail.

### 3. Allocation Status
- allocations have a `status` field.
- **Current Support**: "allocated", "checked_in".
- **Frontend**: "checked_in" allocations should be visually distinct (e.g., green checkmark).

### 4. Event Lock Status
- **"draft"**: Event setup (not relevant for mapping).
- **"allocating"**: Room mapping is ACTIVE. Allocations can be created/modified.
- **"rooms_finalized"**: Room mapping is LOCKED. No changes allowed. Read-only access for Agent/Head Guest.

---

## 📌 1. GET /events/:id

**Purpose**: Fetch basic event details and status.

- **Auth**: Yes
- **Roles**: `agent` (owner), `head_guest` (assigned)

### Request
**Path Params**: `id` (UUID)

### Response (200 OK)
```json
{
  "message": "Event Fetched",
  "event": {
    "id": "uuid...",
    "name": "Wedding of X & Y",
    "status": "allocating", // or "rooms_finalized"
    "rooms_inventory": "[...]", // Raw JSON string or object depending on driver
    ...
  }
}
```

### Frontend Notes
- Check `event.status`. If `"rooms_finalized"`, show a lock banner.
- **Lock Detection**: `status === "rooms_finalized"`

---

## 📌 2. GET /events/:id/guests

**Purpose**: List all guests for the event. Used to populate the "Unallocated Families" list.

- **Auth**: Yes
- **Roles**: `agent`, `head_guest`

### Request
**Path Params**: `id` (UUID)

### Response (200 OK)
```json
{
  "guests": [
    {
      "id": "uuid...",
      "family_id": "uuid...",
      "name": "John Doe",
      "type": "Adult",
      "email": "john@example.com",
      ...
    },
    ...
  ]
}
```

---

## 📌 3. GET /events/:id/allocations

**Purpose**: The main data source for the Room Mapping grid. Returns allocations grouped by `family_id`.

- **Auth**: Yes
- **Roles**: `agent`, `head_guest`

### Request
**Path Params**: `id` (UUID)

### Response (200 OK)
```json
{
  "event_id": "uuid...",
  "status": "allocating",
  "rooms_inventory": [
    {
      "room_offer_id": "uuid...",
      "room_name": "Deluxe Room",
      "available": 5, // Current available count
      "max_capacity": 2,
      "price_per_room": 1000
    }
  ],
  "allocations": [
    {
      "family_id": "uuid...",
      "allocation_id": "uuid... (representative)",
      "room_offer_id": "uuid...", // or null if somehow returned? But usually this endpoint returns allocated only
      "room_name": "Deluxe Room",
      "guests": [
        {
          "guest_id": "uuid...",
          "guest_name": "John Doe"
        }
      ]
    }
  ]
}
```

### Frontend Notes
- **Reconstruct Grid**: Use `rooms_inventory` to build the columns/rows. Use `allocations` to place families into rooms.
- **Inventory Sync**: The `available` count here is the source of truth.
- **Locking**: Response includes `status` for convenience.

---

## 📌 4. POST /allocations

**Purpose**: Allocate a WHOLE FAMILY to a Room Offer.

- **Auth**: Yes
- **Roles**: `agent`, `head_guest`
- **Lock Behavior**: Returns 403 if event is locked.

### Request
**Headers**: `Content-Type: application/json`
**Body**:
```json
{
  "event_id": "uuid...",
  "family_id": "uuid...",
  "room_offer_id": "uuid..."
}
```
*Note: `assigned_by` is NOT sent. Backend derives it from token.*

### Response (201 Created)
```json
{
  "message": "Family allocated successfully",
  "family_id": "uuid...",
  "room_offer_id": "uuid...",
  "guests_affected": 2,
  "assigned_mode": "agent_manual" // or "head_guest_manual"
}
```

### Error Responses
- **400 Bad Request**: Invalid UUID or Family size > Room capacity.
- **403 Forbidden**: Event is locked ("rooms_finalized") or User not authorized.
- **404 Not Found**: Event/Room/Guests not found.
- **409 Conflict**: "No rooms available for this type" or "Family already allocated".

### Frontend Notes
- **Action**: Call checks strict capacity and inventory.
- **On Success**: Remove family from sidebar, add to grid, decrement inventory UI.

---

## 📌 5. PUT /allocations/:id

**Purpose**: Move an already allocated family to a different room type.

- **Auth**: Yes
- **Roles**: `agent`, `head_guest`
- **Lock Behavior**: Returns 403 if event is locked.

### Request
**Path Params**: `id` (Allocation UUID - use any ID from the family's allocations)
**Body**:
```json
{
  "room_offer_id": "uuid..." // The NEW room offer ID
}
```

### Response (200 OK)
```json
{
  "message": "Allocation updated successfully",
  "allocation_id": "uuid...",
  "family_id": "uuid...",
  "old_room_offer_id": "uuid...",
  "new_room_offer_id": "uuid...",
  "assigned_mode": "agent_manual_update", // or "guest_manual_update"
  "guests_affected": 2
}
```

### Error Responses
- **403 Forbidden**: Event locked.
- **409 Conflict**: New room type has no inventory.
- **400 Bad Request**: New room capacity too small for family.

### Frontend Notes
- **Inventory Update**: Backend swaps inventory (Old++, New--). Frontend must reflect this.

---

## 📌 6. POST /events/:id/auto-allocate

**Purpose**: Automatically assign rooms to all unallocated families using a greedy algorithm.

- **Auth**: Yes
- **Roles**: `agent` (Head Guest usually shouldn't, but currently allowed if owner check passes)
- **Lock Behavior**: Returns 403 if event is locked.

### Request
**Path Params**: `id` (Event UUID)
**Body**: `{}` (Empty)

### Response (200 OK)
```json
{
  "message": "Auto-allocation completed",
  "allocations_count": 5, // Number of families allocated
  "guests_count": 12,
  "inventory_updated": true
}
```

### Frontend Notes
- **Trigger**: "Auto Allocate" button.
- **On Success**: **Refetch EVERYTHING** (Guests + Allocations). The grid will change significantly.

---

## 📌 7. POST /events/:id/finalize

**Purpose**: Lock the room mapping. Prevents further changes.

- **Auth**: Yes
- **Roles**: `agent`, `head_guest`

### Request
**Path Params**: `id` (Event UUID)

### Response (200 OK)
```json
{
  "message": "Room mapping locked successfully",
  "status": "rooms_finalized",
  "event_id": "uuid..."
}
```

### Error Responses
- **400 Bad Request**: "Rooms are already finalized".

### Frontend Notes
- On success, disable all drag-and-drop, hide "Auto Allocate", show "Locked" banner.

---

## 📌 8. POST /events/:id/reopen

**Purpose**: Unlock the room mapping.

- **Auth**: Yes
- **Roles**: `agent` ONLY (Head Guest cannot reopen).

### Request
**Path Params**: `id` (Event UUID)

### Response (200 OK)
```json
{
  "message": "Room mapping reopened successfully",
  "status": "allocating",
  "event_id": "uuid..."
}
```

### Frontend Notes
- On success, re-enable UI.

---

## 🧠 assigned_mode Values

| Value | Meaning |
| :--- | :--- |
| `agent_manual` | Allocated manually by Agent (POST /allocations) |
| `head_guest_manual` | Allocated manually by Head Guest (POST /allocations) |
| `agent_manual_update` | Moved to new room by Agent (PUT /allocations) |
| `guest_manual_update` | Moved to new room by Head Guest (PUT /allocations) |
| `auto_assign` | System generated assignment (Auto-Allocate) |

## 🔐 Lock User Experience

1.  **Detection**: Check `GET /events/:id` -> `status`.
2.  **UI State**:
    -   If `status === "rooms_finalized"`:
        -   Show banner: "Room mapping is locked. Try contacting agent."
        -   Disable "Auto Allocate" button.
        -   Disable Drag & Drop.
        -   Show "Reopen" button ONLY if user is Agent.
3.  **API Enforced**: All mutation APIs will strictly return `403` if locked.

## ⚠️ Edge Cases

-   **Capacity Exceeded**: Returns 400. Frontend should prevent drop if `guests.length > room.max_capacity`.
-   **Inventory 0**: Returns 409. Frontend should prevent drop if `available == 0`.
-   **Race Conditions**: Handled by DB allocation. If two users allocate last room efficiently, one gets 409.
-   **Partial Failure**: Transactions ensure atomic updates. Inventory and Allocation always match.
