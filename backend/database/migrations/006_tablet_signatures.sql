ALTER TABLE patient_consents DROP CONSTRAINT IF EXISTS patient_consents_signature_method_check;
ALTER TABLE patient_consents ADD CONSTRAINT patient_consents_signature_method_check CHECK(signature_method IS NULL OR signature_method IN ('tableta','otp','biometrica'));
