package controllers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

func UploadClinicalFile(c *fiber.Ctx) error {
	patientID, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de paciente inválido")
	}
	file, err := c.FormFile("file")
	if err != nil || file.Size < 1 || file.Size > 15*1024*1024 {
		return fail(c, 400, "Adjunta un archivo de hasta 15 MB")
	}
	input, err := file.Open()
	if err != nil {
		return fail(c, 400, "No se pudo abrir el archivo")
	}
	defer input.Close()
	raw, err := io.ReadAll(io.LimitReader(input, 15*1024*1024+1))
	if err != nil || len(raw) < 1 || len(raw) > 15*1024*1024 {
		return fail(c, 400, "No se pudo leer el archivo o excede el tamaño permitido")
	}
	mime := http.DetectContentType(raw[:min(len(raw), 512)])
	allowed := map[string]bool{"image/jpeg": true, "image/png": true, "application/pdf": true}
	if !allowed[mime] {
		return fail(c, 415, "Solo se permiten imágenes PNG/JPEG y PDF; DICOM 3D queda fuera de esta fase")
	}
	encrypted, err := dataCipher.Encrypt(raw)
	if err != nil {
		return fail(c, 500, "No se pudo cifrar el archivo clínico")
	}
	sum := sha256.Sum256(raw)
	random := make([]byte, 24)
	if _, err = rand.Read(random); err != nil {
		return fail(c, 500, "No se pudo asignar identificador seguro")
	}
	name := hex.EncodeToString(random) + ".enc"
	root := os.Getenv("CLINICAL_STORAGE_DIR")
	if root == "" {
		root = "var/clinical"
	}
	if root, err = filepath.Abs(root); err != nil {
		return fail(c, 500, "No se pudo resolver almacenamiento clínico")
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return fail(c, 500, "No se pudo preparar almacenamiento clínico")
	}
	path := filepath.Join(root, name)
	if err = os.WriteFile(path, encrypted, 0600); err != nil {
		return fail(c, 500, "No se pudo guardar el archivo cifrado")
	}
	var id int64
	err = repo.QueryRow(`INSERT INTO clinical_files(patient_id,uploaded_by,original_name,content_type,encrypted_path,sha256) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, patientID, userID(c), filepath.Base(file.Filename), mime, path, hex.EncodeToString(sum[:])).Scan(&id)
	if err != nil {
		_ = os.Remove(path)
		return fail(c, 500, "No se pudo registrar el archivo")
	}
	audit(c, "subir", "archivo_clinico", strconv.FormatInt(id, 10))
	return c.Status(201).JSON(fiber.Map{"id": id, "name": filepath.Base(file.Filename), "content_type": mime, "sha256": hex.EncodeToString(sum[:])})
}

func ListClinicalFiles(c *fiber.Ctx) error {
	patientID, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de paciente inválido")
	}
	rows, err := repo.Query(`SELECT id,original_name,content_type,sha256,created_at FROM clinical_files WHERE patient_id=$1 ORDER BY created_at DESC`, patientID)
	if err != nil {
		return fail(c, 500, "No se pudieron consultar archivos")
	}
	defer rows.Close()
	out := []fiber.Map{}
	for rows.Next() {
		var id int64
		var name, mime, hash string
		var created any
		if rows.Scan(&id, &name, &mime, &hash, &created) == nil {
			out = append(out, fiber.Map{"id": id, "name": name, "content_type": mime, "sha256": hash, "created_at": created})
		}
	}
	audit(c, "consultar", "archivos_clinicos", strconv.FormatInt(patientID, 10))
	return c.JSON(out)
}

func DownloadClinicalFile(c *fiber.Ctx) error {
	id, err := idFromParam(c, "id")
	if err != nil {
		return fail(c, 400, "ID de archivo inválido")
	}
	var name, mime, path string
	var encrypted []byte
	if err = repo.QueryRow(`SELECT original_name,content_type,encrypted_path FROM clinical_files WHERE id=$1`, id).Scan(&name, &mime, &path); err != nil {
		return fail(c, 404, "Archivo no encontrado")
	}
	encrypted, err = os.ReadFile(path)
	if err != nil {
		return fail(c, 404, "Contenido de archivo no disponible")
	}
	plain, err := dataCipher.Decrypt(encrypted)
	if err != nil {
		return fail(c, 500, "No se pudo descifrar el archivo")
	}
	audit(c, "descargar", "archivo_clinico", strconv.FormatInt(id, 10))
	c.Set("Content-Type", mime)
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(name)))
	c.Set("Cache-Control", "no-store")
	return c.Send(plain)
}
