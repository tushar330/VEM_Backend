-- Migration: Family-Based Allocation System
-- Description: Add family_id to guests, remove virtual_room_id, convert IDs to UUID, add constraints

-- Step 1: Add family_id to guests table
ALTER TABLE guests 
ADD COLUMN family_id UUID NOT NULL DEFAULT gen_random_uuid();

-- Step 2: Remove virtual_room_id from guest_allocations
ALTER TABLE guest_allocations 
DROP COLUMN IF EXISTS virtual_room_id;

-- Step 3: Convert room_offers.id to UUID (if it's currently int8)
-- First, check if the column is not already UUID
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'room_offers' 
        AND column_name = 'id' 
        AND data_type != 'uuid'
    ) THEN
        -- Drop foreign key constraints first
        ALTER TABLE guest_allocations DROP CONSTRAINT IF EXISTS fk_guest_allocations_room_offer;
        
        -- Create new UUID column
        ALTER TABLE room_offers ADD COLUMN id_new UUID DEFAULT gen_random_uuid();
        
        -- Update existing records with new UUIDs
        UPDATE room_offers SET id_new = gen_random_uuid() WHERE id_new IS NULL;
        
        -- Drop old id column and rename new one
        ALTER TABLE room_offers DROP COLUMN id CASCADE;
        ALTER TABLE room_offers RENAME COLUMN id_new TO id;
        
        -- Set as primary key
        ALTER TABLE room_offers ADD PRIMARY KEY (id);
    END IF;
END $$;

-- Step 4: Convert guest_allocations.id to UUID
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 
        FROM information_schema.columns 
        WHERE table_name = 'guest_allocations' 
        AND column_name = 'id' 
        AND data_type != 'uuid'
    ) THEN
        -- Create new UUID column
        ALTER TABLE guest_allocations ADD COLUMN id_new UUID DEFAULT gen_random_uuid();
        
        -- Update existing records
        UPDATE guest_allocations SET id_new = gen_random_uuid() WHERE id_new IS NULL;
        
        -- Drop old id and rename
        ALTER TABLE guest_allocations DROP COLUMN id CASCADE;
        ALTER TABLE guest_allocations RENAME COLUMN id_new TO id;
        
        -- Set as primary key
        ALTER TABLE guest_allocations ADD PRIMARY KEY (id);
    END IF;
END $$;

-- Step 5: Ensure room_offer_id in guest_allocations is UUID type
ALTER TABLE guest_allocations 
ALTER COLUMN room_offer_id TYPE UUID USING room_offer_id::UUID;

-- Step 6: Re-add foreign key constraint for room_offer_id
ALTER TABLE guest_allocations 
ADD CONSTRAINT fk_guest_allocations_room_offer 
FOREIGN KEY (room_offer_id) REFERENCES room_offers(id) ON DELETE SET NULL;

-- Step 7: Add UNIQUE constraint on (event_id, guest_id)
ALTER TABLE guest_allocations 
ADD CONSTRAINT unique_event_guest 
UNIQUE (event_id, guest_id);

-- Step 8: Add max_capacity to room_offers if not exists
ALTER TABLE room_offers 
ADD COLUMN IF NOT EXISTS max_capacity INT NOT NULL DEFAULT 2;

-- Step 9: Add status column to guest_allocations with proper values
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'guest_allocations' AND column_name = 'status'
    ) THEN
        ALTER TABLE guest_allocations ADD COLUMN status VARCHAR(50) DEFAULT 'allocated';
    END IF;
END $$;

-- Update status column to use proper enum values
UPDATE guest_allocations 
SET status = 'allocated' 
WHERE status IN ('pending', 'confirmed');

-- Step 10: Update assigned_mode column values
UPDATE guest_allocations 
SET assigned_mode = 'agent_manual' 
WHERE assigned_mode = 'manual';

-- Step 11: Create index on family_id for better query performance
CREATE INDEX IF NOT EXISTS idx_guests_family_id ON guests(family_id);
CREATE INDEX IF NOT EXISTS idx_guests_event_family ON guests(event_id, family_id);

-- Step 12: Add comment for documentation
COMMENT ON COLUMN guests.family_id IS 'UUID identifying the family group. One family = one room allocation.';
COMMENT ON CONSTRAINT unique_event_guest ON guest_allocations IS 'Ensures each guest can only be allocated once per event.';
