Multi2FA TOTP for Windows

Run Multi2FA-TOTP.exe. Vaults are stored in %AppData%\Multi2FA\.

Camera scanning needs ffmpeg: put ffmpeg.exe next to Multi2FA-TOTP.exe
(or into an ffmpeg\bin\ folder next to it).

multi2fa-cli.exe is the command line tool for the same vaults, e.g.
    multi2fa-cli.exe codes
    multi2fa-cli.exe code github -copy
Run "multi2fa-cli.exe -h" for all commands.

https://github.com/keklick1337/Multi2FA-TOTP
