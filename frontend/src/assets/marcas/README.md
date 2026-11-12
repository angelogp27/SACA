# Logos de marcas de hardware

Imágenes que la página usa para identificar el hardware de cada nodo en las
pantallas del clúster (procesador AMD Ryzen o Intel, GPU NVIDIA).

- Formato preferido: SVG (se ve nítido en cualquier tamaño); si no, PNG con
  fondo transparente, de al menos 256 px de ancho.
- Archivos actuales: `Ryzen.svg` (fondo oscuro propio: sus letras son blancas), `Intel.png` y `NVIDIA.png` (recortados a 512 px).
- Se importan desde `src/components/flujo/FlujoCluster.jsx`; Vite las empaqueta
  al compilar la página.
