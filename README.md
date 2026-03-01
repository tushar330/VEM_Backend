# 🏨 TBO Backend

> **A production-grade REST API** for managing corporate event bookings — handling guests, hotels, room allocations, flights, transfers, cart management, and multi-party price negotiation.

---

## 📋 Table of Contents

- [Overview](#overview)
- [Tech Stack](#tech-stack)
- [Architecture](#architecture)
- [Data Models](#data-models)
- [Authentication & Roles](#authentication--roles)
- [API Routes Reference](#api-routes-reference)
- [Key Features](#key-features)
- [Environment Variables](#environment-variables)
- [Getting Started](#getting-started)
- [Project Structure](#project-structure)

---

## Overview

TBO Backend serves as the core API layer for a **corporate travel & event management platform**. An **Agent** creates and manages events, invites guests, browses hotels, manages a cart of rooms/banquets/catering/flights, and negotiates pricing with a TBO Agent before finalizing bookings.

**Core workflow:**

```
Agent Signup → Create Event → Add Guests → Browse Hotels
    → Add to Cart → Start Negotiation → Lock Deal → Finalize Rooms
```

---

## Tech Stack

| Layer | Technology |
|---|---|
| **Language** | Go 1.25 |
| **Web Framework** | [Fiber v2](https://github.com/gofiber/fiber) |
| **ORM** | [GORM](https://gorm.io) with `gorm.io/datatypes` for JSONB |
| **Database** | PostgreSQL (via `pgx` driver) |
| **Caching** | Redis (`go-redis/v9`) |
| **Task Queue** | [Asynq](https://github.com/hibiken/asynq) (backed by Redis) |
| **Authentication** | JWT (`golang-jwt/jwt/v5`) |
| **Password Hashing** | bcrypt (`golang.org/x/crypto`) |
| **ID Generation** | UUID v4 (`google/uuid`) |
| **Config** | `godotenv` + `os.Getenv` |
| **Testing** | `testify` |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Client (Frontend)                     │
└─────────────────────┬───────────────────────────────────────┘
                      │  HTTP/REST
┌─────────────────────▼───────────────────────────────────────┐
│                    Fiber HTTP Server                         │
│  ┌─────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │  CORS MW    │  │  Auth Middleware │  │  Logger/Recover │  │
│  └─────────────┘  └─────────────────┘  └─────────────────┘  │
│                                                              │
│  ┌───────────────────────────────────────────────────────┐   │
│  │                      Handlers                         │   │
│  │  Auth │ Events │ Guests │ Hotels │ Cart │ Negotiation │   │
│  │  Flights │ Transfers │ Allocations │ Locations │ ...  │   │
│  └───────────────────────────────────────────────────────┘   │
└─────────────┬─────────────────────────┬────────────────────-─┘
              │                         │
   ┌──────────▼──────────┐   ┌──────────▼──────────┐
   │      PostgreSQL      │   │        Redis         │
   │  (Primary Data Store)│   │ (Cache + Task Queue) │
   └──────────────────────┘   └──────────────────────┘
                                        │
                              ┌──────────▼──────────┐
                              │   Asynq Worker       │
                              │  (Email Delivery)    │
                              └──────────────────────┘
```

### Key Architectural Decisions

- **Repository Pattern** — All handlers are methods on `handlers.Repository`, which holds `*gorm.DB`, `*asynq.Client`, and config.
- **Redis Caching** — Event listings and individual event details are cached with 1-hour TTL; cache is explicitly invalidated on mutations.
- **Task Queue** — Email delivery (head guest credentials, invitations) is offloaded to Asynq workers running in a background goroutine.
- **JSONB for Flexibility** — `RoomsInventory`, `Facilities`, `ImageUrls`, `Policies`, and `ProposalSnapshot` are stored as PostgreSQL `jsonb` columns for schema-free flexibility.
- **Transactional Deletes** — Cascading deletes (guests → allocations → negotiation sessions → cart items → event) are handled in explicit DB transactions.
- **Polymorphic Cart** — `CartItem` uses a `Type + RefID` pattern to reference rooms, banquets, catering, flights, and transfers from a single table.

---

## Data Models

### Core Entities

```
Users ──────── AgentProfile (1:1)
  │
  ├── Events ─── Guests ──── GuestAllocations ─── RoomOffers
  │       │                                            │
  │       ├── CartItems ────── FlightBookings      Hotels ─── BanquetHalls
  │       │       │            TransferBookings        │
  │       │       └── (rooms, banquets,               └── CateringMenus
  │       │             catering, flights,
  │       └── NegotiationSessions ── NegotiationRounds
  │                  (per CartItem ProposalSnapshot)
  │
Countries ─── Cities ─── Hotels
                  │
              Flights / Transfers (global catalog)
```

### Model Summary

| Model | Key Fields |
|---|---|
| `User` | `id`, `name`, `email`, `role` (`agent`/`head_guest`/`tbo_agent`) |
| `AgentProfile` | `agency_name`, `agency_code`, `location`, `business_phone` |
| `Event` | `name`, `hotel_id`, `location`, `start_date`, `end_date`, `budget`, `status`, `rooms_inventory` (JSONB) |
| `Guest` | `guest_name`, `age`, `type` (`adult`/`child`), `email`, `phone`, `family_id`, `arrival_date`, `departure_date` |
| `GuestAllocation` | `event_id`, `guest_id`, `room_offer_id`, `locked_price`, `status`, `assigned_mode` |
| `Hotel` | `hotel_code`, `name`, `star_rating`, `facilities` (JSONB), `image_urls` (JSONB), `policies` (JSONB) |
| `RoomOffer` | `name`, `booking_code`, `max_capacity`, `total_fare`, `is_refundable`, `cancel_policies` (JSONB) |
| `BanquetHall` | `name`, `capacity`, `price_per_day`, `hall_type`, dimensions, `features` (JSONB) |
| `CateringMenu` | `name`, `type` (`veg`/`non-veg`), `price_per_plate`, `dietary_tags` (JSONB) |
| `CartItem` | `type`, `ref_id`, `status` (`wishlist`/`cart`/`approved`/`booked`), `locked_price`, `quantity` |
| `NegotiationSession` | `event_id`, `status`, `share_token` (UUID), `current_round` |
| `NegotiationRound` | `session_id`, `round_number`, `modified_by`, `proposal_snapshot` (JSONB), `remarks` |

---

## Authentication & Roles

### Auth Mechanism

- **JWT Bearer Tokens** — All protected routes require `Authorization: Bearer <token>` header.
- Tokens are signed with a secret and carry `user_id`, `email`, and `role` as claims.
- The `Protected` middleware validates the token and injects claims into `c.Locals`.

### Roles

| Role | Description | Key Capabilities |
|---|---|---|
| `agent` | Travel agent (signs up publicly) | Create events, manage guests, browse hotels, manage cart, start negotiations |
| `head_guest` | The main guest of an event (created by agent) | View their event, register family members via invite link |
| `tbo_agent` | TBO platform administrator | View all negotiations, submit counter-offers, lock deals |

### Auth Flow

```
[Agent]
  POST /api/v1/auth/signup  →  Creates User + AgentProfile  →  Returns JWT
  POST /api/v1/auth/login   →  Verifies bcrypt hash         →  Returns JWT + User info

[Head Guest]
  POST /api/v1/auth/login   →  Same endpoint                →  Returns JWT + eventId

[TBO Agent]
  Created via seeder script  →  role: "tbo_agent"
```

> **Head Guest creation:** When an agent assigns a head guest (`POST /events/:id/head-guest`), a new `User` with role `head_guest` is created and temporary credentials are emailed via the Asynq queue.

---

## API Routes Reference

**Base URL:** `http://localhost:8080/api/v1`

### 🔓 Public Routes

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/health` | Health check |
| `POST` | `/auth/signup` | Register a new agent account |
| `POST` | `/auth/login` | Login (any role) → returns JWT |
| `POST` | `/agents/onboarding` | Legacy alias for signup |
| `GET` | `/locations/countries` | List all countries |
| `GET` | `/locations/cities` | List all cities (with filtering) |

### 🔐 Protected Routes (JWT Required)

#### 👤 User

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/me` | Get current authenticated user profile |
| `GET` | `/dashboard/metrics` | Get dashboard metrics |

#### 📅 Events

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/events` | List agent's events (with metrics: guest count, budget spent, days until) |
| `POST` | `/events` | Create a new event |
| `GET` | `/events/:id` | Get single event with full metrics |
| `PUT` | `/events/:id` | Update event details |
| `DELETE` | `/events/:id` | Delete event (cascading: guests, allocations, cart, negotiations) |
| `GET` | `/events/:id/guests` | List all guests for an event |
| `POST` | `/events/:id/guests` | Add a guest to an event |
| `GET` | `/events/:id/venues` | Get hotels added to this event's cart |
| `GET` | `/events/:id/allocations` | Get room allocations for the event |
| `POST` | `/events/:id/head-guest` | Assign/create head guest (sends email with credentials) |
| `POST` | `/events/:id/send-invites` | Send invitation emails to guests |
| `POST` | `/events/:id/auto-allocate` | Automatically allocate guests to rooms by family groups |
| `POST` | `/events/:id/finalize` | Finalize room allocations (lock rooms) |
| `POST` | `/events/:id/reopen` | Reopen a finalized allocation |

#### 🛒 Cart

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/events/:id/cart` | Get cart (grouped by hotel → rooms, banquets, catering) |
| `POST` | `/events/:id/cart` | Add a single item to cart |
| `POST` | `/events/:id/cart/bulk` | Bulk add multiple items |
| `PATCH` | `/events/:id/cart/:cartItemId` | Update a cart item (quantity, notes, price) |
| `DELETE` | `/events/:id/cart/:cartItemId` | Remove a cart item |
| `DELETE` | `/events/:id/cart/hotel/:hotelId` | Remove all items for a hotel from cart |
| `POST` | `/events/:id/cart/approve` | Update cart status (wishlist → cart → approved) |

#### 👥 Guests

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/guests/:id` | Get single guest |
| `PATCH` | `/guests/:id` | Update guest details (name, age, email, phone, dates) |
| `DELETE` | `/guests/:id` | Delete a guest |
| `POST` | `/guests/:id/subguests` | Add a family member (sub-guest) to existing guest |

#### 🏠 Allocations

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/allocations` | Manually create a room allocation |
| `PUT` | `/allocations/:id` | Update an allocation |

#### 🏨 Hotels

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/hotels` | List hotels by city (with filters: star rating, property type, user rating) |
| `GET` | `/hotels/:id` | Get hotel details |
| `GET` | `/hotels/:hotelCode/rooms` | Get room offers for a hotel |
| `GET` | `/hotels/:hotelCode/banquets` | Get banquet halls for a hotel |
| `GET` | `/hotels/:hotelCode/catering` | Get catering menus for a hotel |

#### ✈️ Flights

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/flights` | List all global flights |
| `GET` | `/flights/:id` | Get a single flight |
| `POST` | `/flights` | Create a flight (admin) |
| `PUT` | `/flights/:id` | Update a flight |
| `DELETE` | `/flights/:id` | Delete a flight |
| `GET` | `/flights/locations` | Get unique booking locations |
| `GET` | `/events/:id/flights` | Get flight bookings for an event |
| `POST` | `/events/:id/flights/book` | Book a flight for an event |
| `DELETE` | `/events/:id/flights/:booking_id` | Cancel a flight booking |

#### 🚌 Transfers

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/transfers` | List all global transfers |
| `GET` | `/transfers/:id` | Get a single transfer |
| `POST` | `/transfers` | Create a transfer (admin) |
| `PUT` | `/transfers/:id` | Update a transfer |
| `DELETE` | `/transfers/:id` | Delete a transfer |
| `GET` | `/events/:id/transfers` | Get transfer bookings for an event |
| `POST` | `/events/:id/transfers/book` | Book a transfer for an event |
| `DELETE` | `/events/:id/transfers/:booking_id` | Cancel a transfer booking |

#### 🤝 Negotiation

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| `POST` | `/negotiation/init` | Protected | Start negotiation from event cart |
| `POST` | `/negotiation/counter` | Public (share token) | Submit counter offer |
| `GET` | `/negotiation/:id/diff` | Public | Get price diff between rounds |
| `POST` | `/negotiation/lock` | Protected | Lock deal and apply final prices to cart |
| `GET` | `/negotiation/token/:token` | Public | Resolve share token to session |

#### 🛡️ Admin (TBO Agent Only)

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/admin/negotiations` | List all active (non-locked) negotiation sessions |

---

## Key Features

### 🔄 Event Lifecycle Management
Events flow through statuses: `active` → `locked` → `finalized`. Each event carries a `rooms_inventory` JSONB field tracking available/total rooms by type (Single, Double, Triple, Quad).

### 👨‍👩‍👧 Family-Based Guest Allocation
Guests are grouped by `family_id`. The auto-allocate engine assigns rooms based on family size (1 guest → single, 2 → double, etc.) and available room inventory.

### 🛒 Polymorphic Cart
`CartItem` uses a `type + ref_id` design to hold rooms, banquets, catering menus, flights, and transfers in one table. The cart response API returns items grouped hierarchically by hotel.

### 🤝 Multi-Round Price Negotiation
A `NegotiationSession` has multiple `NegotiationRound`s, each storing a full `ProposalSnapshot` (JSONB). The session is shareable via a UUID `share_token`, allowing hotel-side access without authentication. When an agent initiates, a round 1 proposal is created; TBO and agent can counter until the `LockDeal` endpoint applies final prices back to the cart.

### 📧 Async Email Queue
Head guest credential emails and event invitations are processed by Asynq workers running in background goroutines, backed by Redis. Task type: `email:deliver`.

### ⚡ Redis Caching
Events (list + detail) are cached per agent with 1-hour TTL. Cache is invalidated on create/update/delete operations using key patterns like `events:agent:<uuid>` and `events:id:<uuid>`.

---

## Environment Variables

Create a `.env` file in the project root:

```env
# Server
PORT=8080
ENV=development

# Database (PostgreSQL)
DATABASE_URL=postgres://user:password@localhost:5432/tbo_backend

# Redis
REDIS_URL=redis://127.0.0.1:6379

# CORS
ALLOWED_ORIGINS=http://localhost:3000,https://your-frontend.vercel.app
FRONTEND_URL=http://localhost:3000

# JWT
JWT_SECRET=your-super-secret-key
```

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Server port |
| `ENV` | `development` | Environment name |
| `DATABASE_URL` | — | PostgreSQL connection string |
| `REDIS_URL` | `redis://127.0.0.1:6379` | Redis connection URI |
| `ALLOWED_ORIGINS` | `http://localhost:3000` | Comma-separated CORS origins |
| `FRONTEND_URL` | `http://localhost:3000` | Frontend base URL (used in email links) |
| `JWT_SECRET` | — | Secret for signing JWT tokens |

---

## Getting Started

### Prerequisites

- Go 1.21+
- PostgreSQL
- Redis

### Run Locally

```bash
# 1. Clone the repository
git clone https://github.com/akashtripathi12/TBO_Backend.git
cd TBO_Backend

# 2. Copy and configure environment
cp .env.example .env
# Edit .env with your DB and Redis credentials

# 3. Install dependencies
go mod tidy

# 4. Run database migrations (if using migration scripts)
go run cmd/migrate_lifecycle/main.go

# 5. Seed initial data (optional)
go run cmd/seed/main.go

# 6. Start the server
go run cmd/api/main.go
```

The API will be available at `http://localhost:8080`.

### Run Tests

```bash
go test ./internal/handlers/... -v
```

---

## Project Structure

```
TBO_Backend/
├── cmd/
│   ├── api/                    # Main application entry point
│   ├── seed/                   # Data seeder scripts
│   ├── migrate_lifecycle/      # Migration runners
│   ├── migrate_negotiation/    # Negotiation schema migrations
│   └── ...                     # Various dev/debug utility scripts
│
├── internal/
│   ├── config/                 # Config loading from env vars
│   ├── handlers/               # HTTP handler functions (business logic)
│   │   ├── auth.go             # Signup, Login, GetMe
│   │   ├── events.go           # Event CRUD + head guest, invites, finalize
│   │   ├── guests.go           # Guest CRUD + sub-guests
│   │   ├── family_allocations.go # Auto-allocate + manual allocation
│   │   ├── cart.go             # Cart management (add, bulk, update, approve)
│   │   ├── hotel.go            # Hotel, room, banquet, catering queries
│   │   ├── negotiation.go      # Negotiation lifecycle
│   │   ├── flight.go           # Flight catalog + event bookings
│   │   ├── transfer.go         # Transfer catalog + event bookings
│   │   ├── invites.go          # Invitation email dispatch
│   │   └── location.go         # Country/city lookups
│   │
│   ├── middleware/             # Fiber middleware
│   │   ├── auth.go             # JWT validation (Protected handler)
│   │   ├── cors.go             # CORS configuration
│   │   ├── logger.go           # Request logging
│   │   └── recovery.go         # Panic recovery
│   │
│   ├── models/                 # GORM models
│   │   ├── models.go           # Core: User, Guest, Event, Hotel, Room, Banquet, etc.
│   │   ├── cart_models.go      # CartItem + response DTOs
│   │   ├── negotiation.go      # NegotiationSession, NegotiationRound, ProposalItem
│   │   ├── flight_models.go    # Flight catalog model
│   │   ├── transfer_models.go  # Transfer catalog model
│   │   └── ...
│   │
│   ├── queue/                  # Asynq task definitions & handlers
│   │   ├── tasks.go            # Task types & constructors (email:deliver)
│   │   └── worker.go           # Task handler implementations
│   │
│   ├── routes/
│   │   └── routes.go           # All route registrations
│   │
│   ├── store/                  # DB & Redis initialization
│   ├── utils/                  # JWT, response helpers, cache invalidation
│   └── scripts/                # Internal utility scripts
│
├── migrations/                 # SQL migration files
│   ├── 001_family_allocation.sql
│   ├── 002_cart_items.sql
│   └── 003_negotiation.sql
│
├── docs/
│   └── TBO_Backend.postman_collection.json  # Postman collection
│
├── go.mod
├── go.sum
└── .env
```

---

## Response Format

All endpoints return a consistent JSON structure:

```json
// Success
{
  "message": "Events Fetched Successfully",
  "events": [...]
}

// Error
{
  "error": "Description of what went wrong"
}
```

HTTP status codes follow REST conventions: `200 OK`, `201 Created`, `400 Bad Request`, `401 Unauthorized`, `404 Not Found`, `409 Conflict`, `500 Internal Server Error`.

---

*Built with ❤️ using Go + Fiber + GORM + PostgreSQL + Redis*