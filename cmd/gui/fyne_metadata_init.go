package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

func init() {
	app.SetMetadata(fyne.AppMetadata{
		ID: "com.twin.networktoolkit",
		Name: "NetworkToolkit",
		Version: "0.0.1",
		Build: 1,
		Icon: &fyne.StaticResource{
	StaticName: "Icon.png",
	StaticContent: []byte{
		137, 80, 78, 71, 13, 10, 26}},
		
		Release: false,
		Custom: map[string]string{
			
		},
		
		
		Migrations: map[string]bool{
        	
        },
		
	})
}

