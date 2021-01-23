// After installing pihub on a raspberry pi on our
// local network, run the following command.

const serverIP = process.env.SERVER_IP;

const axios = require('axios').default;

const chamberConfig = {
	modules: {
		sensor: {
			source: "htg3535ch",
			config: {
				temperature_adc_channel: 0,
				humidity_adc_channel: 1,
				// calibration_adc_channel: 3,
			}
		},
		fan: {
			source: "relay",
			config: { pin: "20" }
		},
		humidifier: {
			source: "relay",
			config: { pin: "26" }
		},
		i2ctest: {
			source: "i2c",
			// default i2c address for an ADS
			config: { address: 0x48 }
		},
		adstest: {
			source: "ads",
			config: {
				// default i2c address for an ADS
				address: 0x48,
				channel_mask: 4, 
			}
		},
	}
};

(async () => {
	try {
		console.log("initializing");
		await axios.post(`http://${serverIP}:3141/initialize`, chamberConfig);
	} catch (error) {
		console.log("init failed", error);
		process.exit(1);
	}

	setAndSchedule("fan", true, false, 3000)
	setAndSchedule("humidifier", true, false, 5000)
	setInterval(async () => {
		try {
			let res = await axios.post("http://192.168.11.3:3141/act", {
				module: "sensor",
				action: "rh",
			});
			console.log("rh", res.data.result);
		} catch (error) {
			console.log("rh failed", error);
			process.exit(1);
		}

		try {
			let res = await axios.post("http://192.168.11.3:3141/act", {
				module: "sensor",
				action: "tf",
			});
			console.log("tf", res.data.result);
		} catch (error) {
			console.log("tf failed", error);
			process.exit(1);
		}

		try {
			let res = await axios.post("http://192.168.11.3:3141/act", {
				module: "adstest",
				action: "read",
			});
			console.log("ads read", res.data.result);
		} catch (error) {
			console.log("read failed", error);
			process.exit(1);
		}
	}, 10000)
})().catch((error) => console.log("unknown error", error));

async function setAndSchedule(modName, state, other, delay) {
	await turnState(modName, state);
	setTimeout(() => setAndSchedule(modName, other, state, delay), delay);
}

async function turnState(modName, state) {
		try {
		console.log("set", modName, state);
		let res = await axios.post("http://192.168.11.3:3141/act", {
			module: modName,
			action: "set",
			config: {high: state}
		});
	} catch (error) {
		console.log(`${state} failed`, error);
		process.exit(1);
	}
}