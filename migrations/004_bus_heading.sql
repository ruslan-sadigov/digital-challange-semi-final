-- Direction of travel in degrees (0 = north, clockwise), from the last two GPS points.
ALTER TABLE bus ADD COLUMN heading DOUBLE PRECISION;
