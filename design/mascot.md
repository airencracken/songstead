# Songstead's jukebox

Generated with the built-in imagegen tool, using Witmoot's Moot Knight as a warmth
reference and Imvault's blue mascot as the primary graphic style reference.
The original transparent PNG is `jukebox-source.png`; the application and public
website use a smaller transparent PNG for practical download sizes.

The 16px and 32px favicons are resized from `internal/web/static/jukebox.png`
with ImageMagick, retaining its transparent background:

```sh
magick internal/web/static/jukebox.png -resize 32x32 -strip internal/web/static/favicon-32.png
magick internal/web/static/jukebox.png -resize 16x16 -strip internal/web/static/favicon-16.png
```

## Generation prompt

Use case: illustration-story. Asset type: Songstead music recommendation app mascot, a transparent cutout for the header and welcome page. Reference image 1 (Witmoot knight) is a warmth and character reference; reference image 2 (Imvault blue photo mascot) is the primary graphic style reference. Create one original friendly anthropomorphic miniature vintage arched jukebox, matching Imvault's clean rounded cartoon forms, dark soft outlines, simple friendly dot eyes and smile, blue body, pale cream song selection window, modest warm brass detailing, small rounded feet and welcoming little arms. Keep the silhouette clearly recognizable as a jukebox with an arched top, selection buttons and a lower speaker grille. A comfortable companion in the Comfyware family. Full character centered, small-size readable, plenty of clear padding, no scenery, no text, no lettering, no musical note symbols, no watermark. Transparent background with actual alpha.
