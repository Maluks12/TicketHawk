-- Repairs accounts created by the invite flow before the role fix.
--
-- The invite set users.role = 'attendee' while setting staff_role correctly.
-- Both the admin console's auth check and AdminOnlyMiddleware gate on
-- users.role = 'admin', so those accounts could complete signup and then not
-- sign in.

UPDATE users
SET role = 'admin'
WHERE staff_role IS NOT NULL
  AND role <> 'admin';

SELECT email, role, staff_role, is_master_admin
FROM users
WHERE staff_role IS NOT NULL OR is_master_admin
ORDER BY email;