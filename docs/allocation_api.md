# Allocation API Documentation

**Base URL**: `http://localhost:8080/api/v1`

This document outlines the allocation-related API endpoints for the TBO Backend.

## 1. Create Allocation (Allocate Family)
Allocates an entire family to a specific room offer. This endpoint performs validation for room capacity, inventory availability, and ensures no family members are already allocated.

- **Endpoint**: `/allocations`
- **Method**: `POST`
- **Authentication**: Required (`Bearer <token>`)
- **Roles Allowed**: `agent`, `head_guest`

### Request Headers
| Header | Value | Description |
|---|---|---|
| `Content-Type` | `application/json` | Required |
| `Authorization` | `Bearer <your_jwt_token>` | Required |

### Request Body
| Field | Type | Required | Description |
|---|---|---|---|
| `event_id` | UUID (String) | Yes | The ID of the event. |
| `family_id` | UUID (String) | Yes | The ID of the family to allocate. |
| `room_offer_id` | UUID (String) | Yes | The ID of the room offer (from inventory). |

**Example Request:**
```json
{
  "event_id": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
  "family_id": "f1e2d3c4-b5a6-9876-5432-10fedcba9876",
  "room_offer_id": "1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d"
}
```

### Response

#### Success (201 Created)
Returns the allocation details.
*Note: Response keys are currently PascalCase (e.g., `ID`, `EventID`) as per Go struct defaults.*

```json
{
  "message": "Family allocated successfully",
  "family_id": "f1e2d3c4-b5a6-9876-5432-10fedcba9876",
  "room_offer_id": "1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d",
  "family_size": 4,
  "allocations": [
    {
      "ID": "uuid-of-allocation",
      "EventID": "a1b2c3d4-e5f6-7890-1234-567890abcdef",
      "GuestID": "uuid-of-guest",
      "RoomOfferID": "1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d",
      "LockedPrice": 5000,
      "Status": "allocated",
      "AssignedMode": "agent_manual" 
    }
  ]
}
```

#### Error Responses

**400 Bad Request** _(Validation or Capacity Issue)_
```json
{
  "error": "Family size (4) exceeds room capacity (2)"
}
```

**403 Forbidden** _(Unauthorized Role)_
```json
{
  "error": "Not authorized for this event"
}
```

**409 Conflict** _(Inventory or Double Allocation)_
```json
{
  "error": "No rooms available for this type"
}
```
*OR*
```json
{
  "error": "One or more family members are already allocated"
}
```

---

## 2. Get Event Allocations
Retrieves the list of existing allocations for an event.

- **Endpoint**: `/events/:eventId/allocations`
- **Method**: `GET`
- **Authentication**: Required

### Response
**Current Status**: Placeholder Implementation (Returns empty 200 OK)

```json
{
    "message": "Get Event Allocations Endpoint",
    "eventId": "..."
}
```

---

## 3. Update Allocation
Updates an existing allocation (e.g., change room).

- **Endpoint**: `/allocations/:id`
- **Method**: `PUT`
- **Authentication**: Required

### Response
**Current Status**: Not Implemented (501)

```json
{
    "error": "Update allocation not yet implemented"
}
```

---

## 4. Pending Implementation (Internal Handlers Available)
The following handlers exist in the codebase (`internal/handlers/family_allocations.go`) but are **not yet exposed** via `routes.go`.

| Function Handler | Intended Purpose |
|---|---|
| `FinalizeRoomsHandler` | Lock the event allocations (`POST /events/:id/finalize-rooms`) |
| `ReopenAllocationHandler` | Reopen a finalized event (`POST /events/:id/reopen`) |
| `CheckInFamilyHandler` | Mark a family as checked in (`POST /events/:id/families/:familyId/check-in`) |
